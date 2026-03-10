-- scenarigo LSP Feature Demo
--
-- Usage:
--   cd internal/lsp/testdata/demo
--   nvim --clean -u NONE -S demo.lua
--
-- Builds a scenario file from scratch using LSP features,
-- then demonstrates hover, definition jump, diagnostics, and formatting.

-- Find the scenarigo binary.
local root = vim.fn.fnamemodify(vim.fn.resolve(debug.getinfo(1, "S").source:sub(2)), ":h")
local repo_root = vim.fn.fnamemodify(root, ":h:h:h:h")
local binary = repo_root .. "/.bin/scenarigo"

if vim.fn.executable(binary) ~= 1 then
  vim.notify("Binary not found: " .. binary .. "\nRun 'make build' first.", vim.log.levels.ERROR)
  return
end

-- ── Settings ──────────────────────────────────────────────────────────
local PAUSE      = 2000  -- ms between steps
local LONG_PAUSE = 3000
local TYPE_DELAY = 60    -- ms per keystroke
local NAV_DELAY  = 300   -- ms between completion menu navigation

-- ── Helper: statusline ───────────────────────────────────────────────
local demo_status = ""
vim.o.laststatus = 2
vim.o.statusline = " %f  %=%{%v:lua.DEMO_STATUS()%} "
function DEMO_STATUS()
  return demo_status
end

local TOTAL_STEPS = 14
local step_num = 0
local function show(msg)
  step_num = step_num + 1
  demo_status = string.format("[%d/%d] %s", step_num, TOTAL_STEPS, msg)
  vim.cmd("redrawstatus")
  vim.cmd("redraw")
end

-- ── Helper: sequential scheduling ────────────────────────────────────
local queue = {}
local function enqueue(fn)
  table.insert(queue, fn)
end

local function run_queue()
  if #queue == 0 then
    demo_status = "Demo complete! Press q to exit."
    vim.cmd("redrawstatus")
    vim.cmd("redraw")
    if vim.fn.has("gui_running") == 0 and not vim.api.nvim_list_uis()[1] then
      -- Headless mode: dump buffer and quit.
      local lines = vim.api.nvim_buf_get_lines(0, 0, -1, false)
      for i, l in ipairs(lines) do
        io.stderr:write(string.format("%3d: %s\n", i, l))
      end
      vim.cmd("qall!")
    else
      vim.keymap.set("n", "q", function()
        vim.cmd("qall!")
      end, { buffer = 0 })
    end
    return
  end
  local fn = table.remove(queue, 1)
  fn(function()
    vim.defer_fn(run_queue, 200)
  end)
end

-- ── Helper: feedkeys ─────────────────────────────────────────────────
local function feedkeys(k)
  vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes(k, true, false, true), "t", false)
end

-- ── Helper: type characters one at a time (in insert mode) ───────────
-- Uses nvim_buf_set_text for synchronous buffer updates so that LSP
-- didChange notifications are triggered immediately via on_bytes.
local function type_chars(text, idx, cb)
  if idx > #text then cb(); return end
  local pos = vim.api.nvim_win_get_cursor(0)
  local row = pos[1] - 1
  local col = pos[2]
  local ch = text:sub(idx, idx)
  vim.api.nvim_buf_set_text(0, row, col, row, col, { ch })
  vim.api.nvim_win_set_cursor(0, { row + 1, col + 1 })
  vim.cmd("redraw")
  vim.defer_fn(function()
    type_chars(text, idx + 1, cb)
  end, TYPE_DELAY)
end

-- ── Shared state for completion ──────────────────────────────────────
-- Saved by lsp_complete, used by complete_accept.
local _complete_items = {}   -- list of {word=...} Vim complete-items
local _complete_row   = 0    -- 0-based row at trigger time
local _complete_start = 0    -- 0-based col: start of typed prefix
local _complete_end   = 0    -- 0-based col: end of typed prefix (cursor)

-- ── Helper: find start of current word in line ──────────────────────
local function word_start_col(line_text, cursor_col)
  local prefix = line_text:sub(1, cursor_col)
  -- Template variable: start after last "." inside {{ (e.g. "{{vars.api" → "api")
  local tmpl = prefix:match(".*{{.*%.()%w*$")
  if tmpl then return tmpl - 1 end
  -- Regular word boundary (after last whitespace)
  local m = prefix:match(".*%s()%S+$")
  if m then return m - 1 end
  return cursor_col - #(prefix:match("%S*$") or "")
end

-- ── Helper: replace typed prefix with completion word ────────────────
-- Always recalculates positions from the current buffer state to avoid
-- stale coordinates after popup dismiss restores original buffer text.
local function apply_completion(row, sc, ec, word, cb)
  -- Recalculate sc and ec from current buffer state.
  local line = vim.api.nvim_buf_get_lines(0, row, row + 1, false)[1] or ""
  local cur_col = vim.api.nvim_win_get_cursor(0)[2]
  local actual_ec = cur_col
  local actual_sc = word_start_col(line, actual_ec)
  -- Clamp to line length.
  local line_len = #line
  if actual_sc > line_len then actual_sc = line_len end
  if actual_ec > line_len then actual_ec = line_len end
  vim.api.nvim_buf_set_text(0, row, actual_sc, row, actual_ec, { word })
  vim.api.nvim_win_set_cursor(0, { row + 1, actual_sc + #word })
  vim.cmd("redraw")
  vim.defer_fn(cb, 300)
end

-- ── Helper: navigate completion popup + accept (stay in insert mode) ─
-- Popup is visual-only. Text insertion is done via nvim_buf_set_text.
-- When a popup is visible, a CompleteDone autocmd ensures the replacement
-- runs after popup dismissal (which restores original text). When no
-- popup is visible (e.g. headless mode), replacement is done directly.
local function complete_accept(count, idx, cb)
  -- Skip if no completion items (e.g. server returned no results).
  if #_complete_items == 0 then cb(); return end
  if idx > count then
    local word = _complete_items[count] and _complete_items[count].word or ""
    local row = _complete_row

    if vim.fn.pumvisible() == 1 then
      -- Popup is active: use CompleteDone autocmd to run after popup closes.
      vim.api.nvim_create_autocmd("CompleteDone", {
        buffer = 0,
        once = true,
        callback = function()
          vim.schedule(function()
            apply_completion(row, 0, 0, word, cb)
          end)
        end,
      })
      vim.api.nvim_select_popupmenu_item(-1, false, true, {})
    else
      -- No popup (headless or popup already gone): replace directly.
      apply_completion(row, 0, 0, word, cb)
    end
    return
  end
  -- Visual-only navigation: highlight item without modifying buffer text.
  if vim.fn.pumvisible() == 1 then
    vim.api.nvim_select_popupmenu_item(idx - 1, false, false, {})
  end
  vim.cmd("redraw")
  vim.defer_fn(function()
    complete_accept(count, idx + 1, cb)
  end, NAV_DELAY)
end

-- ── Helper: trigger LSP completion synchronously and show popup ──────
-- Must be called in insert mode. Sends request, gets items, shows popup.
local function lsp_complete(cb)
  -- Flush pending didChange notifications by processing the event loop.
  -- Longer wait gives the server time to process the change notification.
  vim.wait(200, function() return false end)

  local pos = vim.api.nvim_win_get_cursor(0)
  local row = pos[1] - 1
  local col = pos[2]
  local line = vim.api.nvim_buf_get_lines(0, row, row + 1, false)[1] or ""

  local params = {
    textDocument = vim.lsp.util.make_text_document_params(0),
    position = { line = row, character = col },
  }
  local results = vim.lsp.buf_request_sync(0, "textDocument/completion", params, 5000)
  if not results then cb(); return end

  for _, res in pairs(results) do
    if res.result then
      local items = res.result.items or res.result
      if type(items) == "table" and #items > 0 then
        -- Compute start_col: prefer server's textEdit, fall back to client-side.
        local start_col = col
        for _, item in ipairs(items) do
          if item.textEdit and item.textEdit.range then
            local te = item.textEdit.range.start.character
            if te ~= col then start_col = te end
            break
          end
        end
        if start_col == col then
          start_col = word_start_col(line, col)
        end

        -- Convert to Vim complete-items.
        local vitems = {}
        for _, item in ipairs(items) do
          local word = item.label
          if item.textEdit and item.textEdit.newText then
            word = item.textEdit.newText
          elseif item.insertText and item.insertText ~= "" then
            word = item.insertText
          end
          table.insert(vitems, {
            word = word,
            abbr = item.label,
            kind = item.detail or "",
            menu = item.documentation and item.documentation:sub(1, 50) or "",
          })
        end

        -- Sort items so that prefix-matching items come first.
        -- This ensures {c=1} selects the best match even if the server
        -- returned unfiltered results (due to document sync delay).
        local typed_prefix = line:sub(start_col + 1, col):lower()
        if #typed_prefix > 0 then
          table.sort(vitems, function(a, b)
            local am = a.abbr:lower():sub(1, #typed_prefix) == typed_prefix
            local bm = b.abbr:lower():sub(1, #typed_prefix) == typed_prefix
            if am and not bm then return true end
            if not am and bm then return false end
            return a.abbr < b.abbr
          end)
        end

        -- Save state for complete_accept.
        _complete_items = vitems
        _complete_row   = row
        _complete_start = start_col
        _complete_end   = col

        -- Show popup (visual only; start_col 0-based → complete() 1-based).
        vim.fn.complete(start_col + 1, vitems)
        vim.cmd("redraw")
        -- Defer so popup becomes active for nvim_select_popupmenu_item.
        vim.defer_fn(cb, 600)
        return
      end
    end
  end
  -- No results: clear stale state to prevent complete_accept from using old data.
  _complete_items = {}
  cb()
end

-- ── Helper: insert-mode action sequence ──────────────────────────────
-- action: {t = "text"} to type char-by-char,  {c = N} to complete.
-- Exits insert mode at the end.
local function insert_actions(actions, idx, cb)
  if idx > #actions then
    vim.cmd("stopinsert")
    vim.cmd("redraw")
    vim.defer_fn(cb, 300)
    return
  end
  local a = actions[idx]
  if a.t then
    type_chars(a.t, 1, function()
      insert_actions(actions, idx + 1, cb)
    end)
  elseif a.c then
    lsp_complete(function()
      complete_accept(a.c, 1, function()
        insert_actions(actions, idx + 1, cb)
      end)
    end)
  end
end

-- ── Helper: open new line below cursor, run actions ──────────────────
local function next_line(actions, cb)
  -- Insert new line synchronously (no feedkeys).
  local cur = vim.api.nvim_win_get_cursor(0)[1]
  vim.api.nvim_buf_set_lines(0, cur, cur, false, { "" })
  vim.api.nvim_win_set_cursor(0, { cur + 1, 0 })
  vim.cmd("startinsert")
  vim.cmd("redraw")
  vim.defer_fn(function()
    insert_actions(actions, 1, cb)
  end, 200)
end


-- ── Helper: hover ────────────────────────────────────────────────────
local function trigger_hover_at(line, col, cb)
  vim.api.nvim_win_set_cursor(0, { line, col })
  vim.cmd("redraw")
  vim.lsp.buf.hover()
  vim.defer_fn(function()
    for _, win in ipairs(vim.api.nvim_list_wins()) do
      if vim.api.nvim_win_get_config(win).relative ~= "" then
        pcall(vim.api.nvim_win_close, win, true)
      end
    end
    vim.defer_fn(cb, 500)
  end, LONG_PAUSE)
end

-- ── Helper: definition jump → show target → jump back ────────────────
local function definition_jump_back(line, col, cb)
  vim.api.nvim_win_set_cursor(0, { line, col })
  vim.cmd("redraw")
  vim.defer_fn(function()
    local def_params = {
      textDocument = vim.lsp.util.make_text_document_params(0),
      position = { line = line - 1, character = col },
    }
    local results = vim.lsp.buf_request_sync(0, "textDocument/definition", def_params, 5000)
    local jumped = false
    local scenario_bufnr = vim.api.nvim_get_current_buf()
    if results then
      for _, res in pairs(results) do
        if res.result then
          local loc = res.result
          if loc.uri then
            vim.lsp.util.show_document(loc, "utf-8", { focus = true })
            jumped = true
          elseif #loc > 0 then
            vim.lsp.util.show_document(loc[1], "utf-8", { focus = true })
            jumped = true
          end
          if jumped then break end
        end
      end
    end
    vim.cmd("redraw")
    vim.defer_fn(function()
      if jumped then
        vim.cmd("buffer " .. scenario_bufnr)
      end
      vim.cmd("redraw")
      vim.defer_fn(cb, PAUSE)
    end, LONG_PAUSE)
  end, PAUSE)
end

-- ── Helper: set line, type text, exit insert ─────────────────────────
local function demo_type(line, base_text, chars, cb)
  vim.api.nvim_buf_set_lines(0, line - 1, line, false, { base_text })
  vim.cmd("redraw")
  vim.api.nvim_win_set_cursor(0, { line, #base_text })
  vim.cmd("startinsert")
  vim.cmd("redraw")
  vim.defer_fn(function()
    type_chars(chars, 1, function()
      vim.cmd("stopinsert")
      vim.defer_fn(cb, 500)
    end)
  end, 200)
end

-- ── Helper: completion demo on existing line ─────────────────────────
local function demo_completion(line, base_text, chars, nav_count, cb)
  vim.api.nvim_buf_set_lines(0, line - 1, line, false, { base_text })
  vim.cmd("redraw")
  vim.api.nvim_win_set_cursor(0, { line, #base_text })
  vim.cmd("startinsert")
  vim.cmd("redraw")
  vim.defer_fn(function()
    local function trigger()
      lsp_complete(function()
        complete_accept(nav_count, 1, function()
          vim.cmd("stopinsert")
          vim.defer_fn(function()
            vim.cmd("redraw")
            cb()
          end, LONG_PAUSE)
        end)
      end)
    end
    if chars and #chars > 0 then
      type_chars(chars, 1, trigger)
    else
      trigger()
    end
  end, 200)
end

-- ── Setup ────────────────────────────────────────────────────────────
vim.o.number = true
vim.o.signcolumn = "yes"
vim.o.pumheight = 15
vim.o.completeopt = "menu,menuone,noselect"
vim.o.updatetime = 300
vim.o.autoindent = false
vim.o.swapfile = false

vim.diagnostic.config({
  virtual_text = true,
  signs = true,
  underline = true,
  update_in_insert = false,
})

-- Open the scenario file and clear it to start from scratch.
local scenario_file = root .. "/scenario.yaml"
vim.cmd("edit " .. vim.fn.fnameescape(scenario_file))
local bufnr = vim.api.nvim_get_current_buf()
vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, { "" })

-- Start the LSP client.
local client_id = vim.lsp.start({
  name = "scenarigo",
  cmd = { binary, "lsp" },
  root_dir = root,
  init_options = { formatting = true },
})

if not client_id then
  vim.notify("Failed to start LSP server", vim.log.levels.ERROR)
  return
end

vim.wait(5000, function()
  return #vim.lsp.get_clients({ bufnr = bufnr, name = "scenarigo" }) > 0
end, 100)

-- Set omnifunc for LSP completion (used as fallback; demo uses lsp_complete helper).
vim.bo[bufnr].omnifunc = "v:lua.vim.lsp.omnifunc"

-- ══════════════════════════════════════════════════════════════════════
-- Phase 1: Build scenario from scratch
-- ══════════════════════════════════════════════════════════════════════

-- 1. Type schemaVersion line
enqueue(function(next)
  show("Typing schema version")
  vim.defer_fn(function()
    vim.api.nvim_win_set_cursor(0, { 1, 0 })
    vim.cmd("startinsert")
    vim.cmd("redraw")
    vim.defer_fn(function()
      insert_actions({ { t = "schemaVersion: scenario/v1" } }, 1, next)
    end, 200)
  end, PAUSE)
end)

-- 2. Key completion: title
enqueue(function(next)
  show("Key Completion — title")
  vim.defer_fn(function()
    next_line({ { t = "ti" }, { c = 1 }, { t = "Feature Demo" } }, next)
  end, PAUSE)
end)

-- 3. Key completion: plugins, vars, secrets
enqueue(function(next)
  show("Key Completion — plugins, vars & secrets")
  vim.defer_fn(function()
    next_line({ { t = "pl" }, { c = 1 } }, function()
      next_line({ { t = "  myplugin: " }, { c = 1 } }, function()
        next_line({ { t = "va" }, { c = 1 } }, function()
          next_line({ { t = "  apiEndpoint: https://api.example.com" } }, next)
        end)
      end)
    end)
  end, PAUSE)
end)

-- 4. Key completion: steps + step title
enqueue(function(next)
  show("Key Completion — steps")
  vim.defer_fn(function()
    next_line({ { t = "ste" }, { c = 1 } }, function()
      next_line({ { t = "  - title: Login" } }, next)
    end)
  end, PAUSE)
end)

-- 5. Step key + enum completion: protocol: http
enqueue(function(next)
  show("Step Key + Enum Completion — protocol: http")
  vim.defer_fn(function()
    next_line({ { t = "    pro" }, { c = 1 }, { c = 1 } }, next)
  end, PAUSE)
end)

-- 6. Request block + HTTP field + plugin export completion + definition jump
enqueue(function(next)
  show("Plugin Export Completion + Jump → Go source → back")
  vim.defer_fn(function()
    next_line({ { t = "    req" }, { c = 1 } }, function()
      next_line({ { t = "      cl" }, { c = 1 }, { t = "'{{plugins.myplugin." }, { c = 1 }, { t = "(vars.api" }, { c = 1 }, { t = ")}}'" } }, function()
        -- Jump to Go source definition of CreateClient.
        local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
        for li, ll in ipairs(lines) do
          if ll:match("plugins%.myplugin%.") then
            local dot_end = ll:find("myplugin%.")
            if dot_end then
              dot_end = dot_end + #"myplugin."
            end
            local def_col = (dot_end or 21) - 1
            vim.api.nvim_win_set_cursor(0, { li, def_col })
            vim.cmd("redraw")
            vim.wait(1000, function() return false end)
            vim.defer_fn(function()
              local def_params = {
                textDocument = vim.lsp.util.make_text_document_params(0),
                position = { line = li - 1, character = def_col },
              }
              local results = vim.lsp.buf_request_sync(0, "textDocument/definition", def_params, 5000)
              local jumped = false
              if results then
                for _, res in pairs(results) do
                  if res.result then
                    -- Result may be a single Location {uri, range} or an array.
                    local loc = res.result
                    if loc.uri then
                      -- Single Location object.
                      vim.lsp.util.show_document(loc, "utf-8", { focus = true })
                      jumped = true
                    elseif #loc > 0 then
                      vim.lsp.util.show_document(loc[1], "utf-8", { focus = true })
                      jumped = true
                    end
                    if jumped then break end
                  end
                end
              end
              vim.cmd("redraw")
              vim.defer_fn(function()
                if jumped then
                  vim.cmd("buffer " .. bufnr)
                end
                vim.cmd("redraw")
                -- Continue with method and url.
                next_line({ { t = "      me" }, { c = 1 }, { t = "POST" } }, function()
                  next_line({ { t = "      url: '{{vars.api" }, { c = 1 }, { t = "}}/login'" } }, next)
                end)
              end, LONG_PAUSE)
            end, PAUSE)
            return
          end
        end
        -- Fallback: continue without jump.
        next_line({ { t = "      me" }, { c = 1 }, { t = "POST" } }, function()
          next_line({ { t = "      url: '{{vars.api" }, { c = 1 }, { t = "}}/login'" } }, next)
        end)
      end)
    end)
  end, PAUSE)
end)

-- 7. Header + template completion for secrets.token
enqueue(function(next)
  show("HTTP Field + Template Completion — header & secrets")
  vim.defer_fn(function()
    next_line({ { t = "      hea" }, { c = 1 } }, function()
      next_line({ { t = "        Authorization: 'Bearer {{secrets.to" }, { c = 1 }, { t = "}}'" } }, next)
    end)
  end, PAUSE)
end)

-- 8. Definition jump: secrets.token → scenarigo.yaml → back
enqueue(function(next)
  show("Definition Jump — secrets.token → scenarigo.yaml → back")
  local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
  for i, l in ipairs(lines) do
    if l:match("secrets%.token") then
      local col = l:find("token")
      definition_jump_back(i, (col or 16) - 1, next)
      return
    end
  end
  next()
end)

-- 9. Expect block with field key completion
enqueue(function(next)
  show("HTTP Field Completion — expect")
  vim.defer_fn(function()
    next_line({ { t = "    exp" }, { c = 1 } }, function()
      next_line({ { t = "      co" }, { c = 1 }, { t = "200" } }, function()
        next_line({ { t = "      bo" }, { c = 1 } }, function()
          next_line({ { t = "        status: ok" } }, next)
        end)
      end)
    end)
  end, PAUSE)
end)

-- ══════════════════════════════════════════════════════════════════════
-- Phase 2: Feature demos
-- ══════════════════════════════════════════════════════════════════════

-- 11. Hover on protocol
enqueue(function(next)
  show("Hover — field description, type, and enum values")
  vim.defer_fn(function()
    local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
    for i, l in ipairs(lines) do
      if l:match("^    protocol:") then
        trigger_hover_at(i, 6, next)
        return
      end
    end
    next()
  end, PAUSE)
end)

-- 10. Definition jump: global var → scenarigo.yaml → back
enqueue(function(next)
  show("Definition Jump — global var → scenarigo.yaml → back")
  local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
  for i, l in ipairs(lines) do
    if l:match("vars%.apiEndpoint") then
      local col = l:find("apiEndpoint")
      definition_jump_back(i, (col or 16) - 1, next)
      return
    end
  end
  next()
end)


-- 12. Signature Help
enqueue(function(next)
  show("Signature Help — assert function signature")
  vim.defer_fn(function()
    local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
    for i, l in ipairs(lines) do
      if l:match("status: ok") then
        demo_type(i, '        status: "', "{{assert.contains <- ", function()
          local cur_line = vim.api.nvim_buf_get_lines(bufnr, i - 1, i, false)[1]
          vim.api.nvim_win_set_cursor(0, { i, #cur_line - 1 })
          vim.lsp.buf.signature_help()
          vim.defer_fn(function()
            for _, win in ipairs(vim.api.nvim_list_wins()) do
              if vim.api.nvim_win_get_config(win).relative ~= "" then
                pcall(vim.api.nvim_win_close, win, true)
              end
            end
            vim.api.nvim_buf_set_lines(bufnr, i - 1, i, false, { l })
            vim.defer_fn(next, 500)
          end, LONG_PAUSE)
        end)
        return
      end
    end
    next()
  end, PAUSE)
end)

-- 13. Diagnostics — add invalid field
enqueue(function(next)
  show("Diagnostics — unknown field detection")
  vim.defer_fn(function()
    -- Find the expect block and insert an invalid field after it.
    local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
    for i, l in ipairs(lines) do
      if l:match("^    expect:") then
        -- Insert an unknown field "tmeout" (typo of timeout) as a sibling of expect.
        vim.api.nvim_buf_set_lines(bufnr, i - 1, i - 1, false, { "    tmeout: 30s" })
        vim.cmd("redraw")
        -- Wait for LSP to process didChange and return diagnostics.
        vim.defer_fn(function()
          vim.diagnostic.show(nil, bufnr)
          vim.cmd("redraw")
          vim.defer_fn(function()
            -- Remove the invalid line.
            vim.api.nvim_buf_set_lines(bufnr, i - 1, i, false, {})
            vim.defer_fn(next, 1000)
          end, LONG_PAUSE)
        end, 2000)
        return
      end
    end
    next()
  end, PAUSE)
end)

-- 14. Formatting — reorder keys
enqueue(function(next)
  show("Formatting — reorder keys to match schema order")
  vim.defer_fn(function()
    local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
    local sv_line, title_line
    for i, l in ipairs(lines) do
      if l:match("^schemaVersion:") then sv_line = i end
      if l:match("^title:") then title_line = i; break end
    end
    if sv_line and title_line then
      local sv = lines[sv_line]
      local tl = lines[title_line]
      vim.api.nvim_buf_set_lines(bufnr, sv_line - 1, sv_line, false, { tl })
      vim.api.nvim_buf_set_lines(bufnr, title_line - 1, title_line, false, { sv })
      vim.cmd("redraw")
      vim.defer_fn(function()
        vim.lsp.buf.format({ async = false })
        vim.cmd("redraw")
        vim.defer_fn(next, LONG_PAUSE)
      end, PAUSE)
    else
      next()
    end
  end, PAUSE)
end)

-- ── Start ────────────────────────────────────────────────────────────
show("scenarigo LSP Feature Demo — starting...")
vim.defer_fn(run_queue, PAUSE)
