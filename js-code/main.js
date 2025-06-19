const unusedVar = 42

function greet(name) {
  if (name) {
    console.log("Hello, " + name)
  }
  else {
    return
  }
}

greet("world")

function noReturn(x) {
  if (x > 0) {
    return x
  }
  return 0
}