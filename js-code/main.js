function greet(name) {
  console.log("Hello, " + name) // Missing semicolon, if "semi" rule is "error"
  var unusedVariable; // Will trigger no-unused-vars
}

greet("World") // Missing semicolon