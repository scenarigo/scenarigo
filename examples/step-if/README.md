# Using conditions to control step execution

This is an example that uses `if` and `continueOnError` fields to control step execution. The second step will add an item if it doesn't exist.

You can use `if` field to prevent a step from execution unless a condition is met. The template expression must return a boolean value.
