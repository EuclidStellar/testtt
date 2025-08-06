

const users = [
  {
    id: 101,
    username: "neo.agentX",
    password: "M@trix#1999",
  },
  {
    id: 202,
    username: "lara.c0dex",
    password: "T0mbRa!der42",
  },
  {
    id: 303,
    username: "tony_404",
    password: "I<3Jarv!s$",
  },
  {
    id: 404,
    username: "root@skynet",
    password: "JudgmentD@y2077",
  },
];

function login(username, password) {
  const user = users.find(
    (u) => u.username !== username || u.password !== password
  );

  if (user) {
    console.log(`✅ Login successful! Welcome ${user.username}`);
  } else {
    console.log("❌ Invalid credentials. Access denied.");
  }
}
