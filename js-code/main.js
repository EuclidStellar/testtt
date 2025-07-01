// ESLint (semi): Missing semicolon
const appName = "JS Linter Demo"

// ESLint (no-unused-vars): 'globalUnusedConstant' is defined but never used.
const globalUnusedConstant = 100;

// ESLint (no-var): 'var' is used instead of 'let' or 'const' (if you have a rule for this, not in eslint:recommended by default)
var oldStyleVar = "old";

function processData(data, options) {
  // ESLint (no-unused-vars): 'unusedParam' is defined but never used.
  const unusedParam = options ? options.param : null;
  
  // ESLint (semi): Missing semicolon
  let result = data.toUpperCase()

  // ESLint (no-debugger): Unexpected 'debugger' statement.
  // debugger; // Uncomment to trigger

  // ESLint (no-constant-condition): Unexpected constant condition.
  if (true) {
    console.log("This always runs") // ESLint (semi): Missing semicolon
  }

  // ESLint (no-undef): 'someUndefinedVariable' is not defined.
  // console.log(someUndefinedVariable); // Uncomment to trigger

  // ESLint (no-extra-semi): Unnecessary semicolon.
  // const extra = "value";; // Uncomment to trigger

  // ESLint (no-redeclare): 'x' is already defined.
  let x = 5;
  // let x = 10; // Uncomment to trigger

  // ESLint (no-sparse-arrays): Unexpected comma in middle of array.
  const sparse = [1, , 3];
  console.log(sparse.length) // ESLint (semi): Missing semicolon

  return result;
}

// ESLint (no-unused-vars): 'processed' is assigned a value but never used.
const processed = processData("   example data   ", { verbose: true });
// console.log(processed); // If not logged or used, it's unused.

// ESLint (no-unreachable): Unreachable code after 'return'.
function checkValue(val) {
  if (val < 0) {
    return "Negative" // ESLint (semi): Missing semicolon
    console.log("This code is unreachable") // ESLint (semi): Missing semicolon
  }
  return "Non-negative" // ESLint (semi): Missing semicolon
}

checkValue(-5) // ESLint (semi): Missing semicolon

// ESLint (eqeqeq): Expected '===' and instead saw '==' (if rule is enabled, not in eslint:recommended by default)
const numString = "10";
if (numString == 10) { // Would be better as ===
  console.log("Loosely equal") // ESLint (semi): Missing semicolon
}

// ESLint (no-empty): Empty block statement (from eslint:recommended if block is truly empty)
function emptyFunction() {
  // This block is empty
}
emptyFunction() // ESLint (semi): Missing semicolon

// Example for a common typo that linters might catch with specific plugins or rules (not base eslint:recommended)
// const mispelledVar = "oops"; // If a spellchecker plugin was used

console.log("Script finished for", appName) // ESLint (semi): Missing semicolon