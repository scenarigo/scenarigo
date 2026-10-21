-- Neovim headless e2e test for scenarigo LSP server.
--
-- Usage (called from Go e2e test):
--   nvim --headless --clean -u NONE -S lsp_test.lua
--
-- Expects environment variables:
--   SCENARIGO_BINARY  — path to the scenarigo binary
--   SCENARIGO_WORKDIR — workspace directory (test files written here)
--
-- Writes JSON results to $SCENARIGO_WORKDIR/results.json.

local binary_path = vim.env.SCENARIGO_BINARY
local workspace_dir = vim.env.SCENARIGO_WORKDIR

if not binary_path or not workspace_dir then
  io.stderr:write("SCENARIGO_BINARY and SCENARIGO_WORKDIR must be set\n")
  vim.cmd("cquit! 1")
  return
end

local results = {
  tests = {},
  errors = {},
}

local function record(name, ok, detail)
  table.insert(results.tests, { name = name, ok = ok, detail = detail or "" })
end

local function record_error(msg)
  table.insert(results.errors, msg)
end

local function write_results_and_exit()
  local f = io.open(workspace_dir .. "/results.json", "w")
  if f then
    f:write(vim.json.encode(results))
    f:close()
  end
  local has_failure = false
  for _, t in ipairs(results.tests) do
    if not t.ok then
      has_failure = true
      break
    end
  end
  if #results.errors > 0 then
    has_failure = true
  end
  vim.cmd("cquit! " .. (has_failure and "1" or "0"))
end

-- Create the test YAML file on disk.
local test_file = workspace_dir .. "/test.yaml"
local test_content = table.concat({
  "schemaVersion: scenario/v1",
  "title: nvim e2e test",
  "steps:",
  "  - title: step1",
  "    protocol: http",
  "    request:",
  "      method: GET",
  "      url: http://example.com",
  "",
}, "\n")

do
  local f = io.open(test_file, "w")
  if not f then
    record_error("failed to write test file")
    write_results_and_exit()
    return
  end
  f:write(test_content)
  f:close()
end

-- Open the test file in a buffer.
vim.cmd("edit " .. vim.fn.fnameescape(test_file))
local bufnr = vim.api.nvim_get_current_buf()

-- Start the LSP client.
local client_id = vim.lsp.start({
  name = "scenarigo-lsp-e2e",
  cmd = { binary_path, "lsp" },
  root_dir = workspace_dir,
})

if not client_id then
  record_error("vim.lsp.start returned nil")
  write_results_and_exit()
  return
end

record("lsp_start", true)

-- Wait for the client to initialize (poll with vim.wait).
local initialized = vim.wait(5000, function()
  local clients = vim.lsp.get_clients({ bufnr = bufnr, name = "scenarigo-lsp-e2e" })
  return #clients > 0
end, 100)

record("client_attached", initialized, "")

if not initialized then
  record_error("LSP client did not attach within 5s")
  write_results_and_exit()
  return
end

local client = vim.lsp.get_clients({ bufnr = bufnr, name = "scenarigo-lsp-e2e" })[1]

-- Test: Completion at line 8, col 6 (inside request block).
local completion_done = false
local completion_ok = false
local completion_detail = ""

local comp_params = vim.lsp.util.make_position_params(0, "utf-8")
comp_params.position = { line = 8, character = 6 }

client:request("textDocument/completion", comp_params, function(err, result)
  if err then
    completion_detail = "error: " .. vim.inspect(err)
  elseif not result then
    completion_detail = "nil result"
  else
    local items = result.items or {}
    completion_ok = #items > 0
    completion_detail = "items: " .. #items
  end
  completion_done = true
end, bufnr)

vim.wait(5000, function() return completion_done end, 100)
record("completion", completion_ok, completion_detail)

-- Test: Hover on "protocol" (line 4, col 6).
local hover_done = false
local hover_ok = false
local hover_detail = ""

local hover_params = vim.lsp.util.make_position_params(0, "utf-8")
hover_params.position = { line = 4, character = 6 }

client:request("textDocument/hover", hover_params, function(err, result)
  if err then
    hover_detail = "error: " .. vim.inspect(err)
  elseif not result then
    hover_detail = "nil result"
  else
    local value = ""
    if result.contents then
      if type(result.contents) == "string" then
        value = result.contents
      elseif result.contents.value then
        value = result.contents.value
      end
    end
    hover_ok = value ~= ""
    hover_detail = "value length: " .. #value
  end
  hover_done = true
end, bufnr)

vim.wait(5000, function() return hover_done end, 100)
record("hover", hover_ok, hover_detail)

-- Test: Diagnostics for valid document (should be 0).
-- Wait a moment for initial diagnostics to settle.
vim.wait(1000, function()
  return false -- just wait
end, 100)

local diags = vim.diagnostic.get(bufnr)
record("diagnostics_clean", #diags == 0, "count: " .. #diags)

-- Test: Edit buffer to introduce an error and check diagnostics.
vim.api.nvim_buf_set_lines(bufnr, 2, 2, false, { "badKey: value" })

-- Wait for diagnostics to appear.
local found_bad_key = vim.wait(5000, function()
  local d = vim.diagnostic.get(bufnr)
  for _, diag in ipairs(d) do
    if diag.message and diag.message:find("badKey") then
      return true
    end
  end
  return false
end, 100)

local diags_after = vim.diagnostic.get(bufnr)
local messages = {}
for _, d in ipairs(diags_after) do
  table.insert(messages, d.message or "")
end
record("diagnostics_after_edit", found_bad_key,
  "count: " .. #diags_after .. ", messages: " .. table.concat(messages, "; "))

-- Done.
vim.lsp.stop_client(client_id)
vim.wait(1000, function()
  return #vim.lsp.get_clients() == 0
end, 100)

write_results_and_exit()
