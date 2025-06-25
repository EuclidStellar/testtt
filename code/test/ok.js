// This file contains several ESLint errors for testing purposes.

// no-unused-vars: 'unusedVar' is defined but never used.
const unusedVar = 'I am not used';

// no-var: Use 'let' or 'const' instead of 'var'.
var oldStyleVar = 'hello';

function greet(name) {
    // no-console: Unexpected console statement.
    console.log('Hello, ' + name);

    // eqeqeq: Expected '===' and instead saw '=='.
    if (name == 'world') {
        return true;
    }
}

// no-undef: 'nonExistentFunction' is not defined.
// This will cause a runtime error, but ESLint should catch it statically.
try {
    nonExistentFunction();
} catch (e) {
    // ignore
}


// Call the function to avoid another unused-vars error for 'greet'
greet('world');

// Bad formatting (indentation)
    const x = 5;