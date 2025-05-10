package tool

import (
	"fmt"

	"github.com/grafana/sobek"
)

const printDescription = `
## Description
The print function outputs values from your CodeAct JavaScript program back to you. It serves as a communication channel to display information during code execution, helping with debugging, monitoring program state, and returning computation results.
Unlike console.log() which writes to the standard output stream, print specifically communicates back to the agent, ensuring the output is visible in the conversation. ALWAYS use print instead of console.log().

## Parameters
- **value** (*any*, required): The value to output. Can be any JavaScript value:
  - Primitive types (numbers, strings, booleans, etc.)
  - Complex objects and arrays (automatically stringified)
  - Function references
  - Errors

## Expected Output
The print function returns undefined. It's used for its side effect of displaying output, not for its return value.

## CRITICAL REQUIREMENTS
- **Always use print instead of console.log()**: Unlike console.log(), print is specifically designed to communicate back to you.
- **Large Data**: Very large objects or strings might be truncated in the output
- **Performance**: Excessive printing of large objects may impact performance
- **Complex Objects**: Consider whether you need the entire object printed or just specific properties
- **Label Your Outputs**: Include descriptive labels to provide context for the printed values
%[1]s
  print("User data:", userData);
%[1]s
- **Strategic Placement**: Place print statements at critical points in your code:
  - Before and after important operations
  - Inside conditional blocks to confirm which path was taken
  - At function entry and exit points
- **Clear Separation**: Use visual separators for important outputs:
%[1]s
  print("=== PROCESSING RESULTS ===");
  print(results);
%[1]s
- **Contextualize Values**: Provide context for what each printed value represents:
%[1]s
  print("Database query returned ${results.length} records");
%[1]s

## When to Use
- **Debugging**: Display variable values, execution flow, or program state at different points
- **Result Reporting**: Show computation results, statistics, or processed data
- **Verification**: Confirm that file operations, API calls, or data transformations worked as expected
- **Data Exploration**: Examine the structure and content of objects, arrays, or file data

## Common Errors and How to Fix Them
- **Output Not Visible**: Ensure the print statement is actually executed. Check conditional logic or function calls.
- **Truncated Output**: Split large outputs into smaller chunks or print specific properties instead of entire objects.

## Usage Examples
%[1]s
// Basic primitive types
print("Hello, world!");  // String
print(42);               // Number
print(true);             // Boolean

// String templates with variables
const username = "Alice", score = 95;
print("User ${username} scored: ${score}");

// Objects and arrays (automatically stringified)
print({name: "Bob", age: 30, roles: ["admin", "editor"]});
// Output: {"name":"Bob","age":30,"roles":["admin","editor"]}

print(["apple", "banana", "cherry"]);
// Output: ["apple","banana","cherry"]

// Operation results
const numbers = [1, 2, 3, 4, 5];
print("Sum of array: ${numbers.reduce((a, b) => a + b, 0)}");

// File operations
const content = read_file("/path/to/file.txt");
print("File content:", content);
%[1]s
`

func NewPrintTool() CodeActTool {
	return NewOnDemandTool(
		"print",
		fmt.Sprintf(printDescription, "```"),
		printHandler,
	)
}

func printHandler(session *CodeActSession) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		args := make([]interface{}, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}

		fmt.Fprintln(session.System, args...)
		return sobek.Undefined()
	}
}
