
## Description
The print function outputs values from your code_interpreter program to you. Unlike console.log() which writes to the standard output stream, 
print specifically communicates tool results back to the you, meaning it is invisible to the user. ALWAYS use print instead of console.log(). 
Never use print to display results to the user. To share content with the user, write it directly in your response or use create_file to save it. 
The results of tool calls in your code_interpreter program will not be visible to you if you do not print them.

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
- **The user will not see the output of print statements, only you will**: You should not use it to communicate results to the user.
- **Large Data**: Very large objects or strings might be truncated in the output
- **Performance**: Excessive printing of large objects may impact performance
- **Complex Objects**: Consider whether you need the entire object printed or just specific properties
- **Label Your Outputs**: Include descriptive labels to provide context for the printed values
```
  print("User data:", userData);
```
- **Strategic Placement**: Place print statements at critical points in your code:
  - Before and after important operations
  - Inside conditional blocks to confirm which path was taken
  - At function entry and exit points
- **Clear Separation**: Use visual separators for important outputs:
```
  print("=== PROCESSING RESULTS ===");
  print(results);
```
- **Contextualize Values**: Provide context for what each printed value represents:
```
  print("Database query returned ${results.length} records");
```

## When to Use
- **Debugging**: Display variable values, execution flow, or program state at different points
- **Result Reporting**: Show computation results, statistics, or processed data
- **Verification**: Confirm that file operations, API calls, or data transformations worked as expected
- **Data Exploration**: Examine the structure and content of objects, arrays, or file data

## Common Errors and How to Fix Them
- **Output Not Visible**: Ensure the print statement is actually executed. Check conditional logic or function calls.
- **Truncated Output**: Split large outputs into smaller chunks or print specific properties instead of entire objects.

## Usage Examples
```
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
```
