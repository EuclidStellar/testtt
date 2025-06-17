// ESLint: Missing semicolon (semi)
const name = "JavaScript Linter Test"

// ESLint: 'unusedVar' is assigned a value but never used (no-unused-vars)
let unusedVar = 42;

// ESLint: 'undeclaredVar' is not defined (no-undef)
// console.log(undeclaredVar); // Commenting out to allow script to run, but ESLint will flag if uncommented

function greet(person) {
  // ESLint: Expected an assignment or function call and instead saw an expression (no-unused-expressions)
  // person; // Uncomment to see this error

  if (person) {
    // ESLint: 'message' is defined but never used (no-unused-vars)
    let message = "Hello, " + person + "!";
    // console.log(message); // If you don't use message, it's unused
    return "Hello, " + person + "!" // Missing semicolon
  } else {
    // ESLint: Unnecessary `else` after `return` (no-else-return) - if eslint-plugin-no-else-return is used or similar rule
    // This specific rule might not be in eslint:recommended by default but is a common one.
    // For eslint:recommended, this is fine.
    return "Hello, guest!" // Missing semicolon
  }
}

// ESLint: Missing semicolon (semi)
greet("Alice")

// ESLint: Unexpected constant condition (no-constant-condition)
if (true) {
  // ESLint: 'anotherUnused' is assigned a value but never used (no-unused-vars)
  const anotherUnused = "test";
}

// ESLint: 'duplicateVar' is already defined (no-redeclare)
var duplicateVar = 10;
// var duplicateVar = 20; // Uncomment to see this error

// ESLint: Extra semicolon (no-extra-semi)
// const extraSemi = "value";; // Uncomment to see this error

// ESLint: `foo` function is defined but never used (no-unused-vars, if functions are checked)
function foo() {
  // ESLint: `bar` is not defined (no-undef)
  // return bar; // Uncomment to see this error
}

// ESLint: debugger statement (no-debugger) - from eslint:recommended
// debugger; // Uncomment to see this error

// ESLint: sparse arrays (no-sparse-arrays) - from eslint:recommended
const sparseArray = [1, , 3];
console.log(sparseArray) // Missing semicolon

// ESLint: unreachable code after return (no-unreachable)
function checkUnreachable(val) {
  if (val > 5) {
    return true; // Missing semicolon
    console.log("This is unreachable") // Missing semicolon
  }
  return false // Missing semicolon
}
checkUnreachable(10) // Missing semicolon