/**
 * A Node.js script to test the GitHub Models API rate limit.
 * It makes 50 quick calls to exhaust the limit, then makes one final
 * call to display the rate limit error response.
 */

async function testRateLimit() {
  // 1. Get the Personal Access Token from an environment variable for security.
  const pat = process.env.GH_PAT_MODELS;

  if (!pat) {
    console.error('❌ Error: The GH_PAT_MODELS environment variable is not set.');
    console.log('\nPlease set it in your terminal before running the script:');
    console.log('export GH_PAT_MODELS="your_pat_here"');
    process.exit(1); // Exit the script if the token is missing.
  }

  // 2. Define API call details.
  const API_URL = "https://models.github.ai/inference/chat/completions";
  const payload = {
    model: "gpt-4.1-nano",
    messages: [{ "role": "user", "content": "This is a test call." }],
  };
  const headers = {
    'Authorization': `Bearer ${pat}`,
    'Content-Type': 'application/json',
  };

  console.log("🚀 Starting 50 calls to exhaust the rate limit...");

  // 3. Loop 50 times to hit the rate limit.
  for (let i = 1; i <= 50; i++) {
    try {
      // We only care about the status, so we don't need to read the body.
      const response = await fetch(API_URL, {
        method: 'POST',
        headers: headers,
        body: JSON.stringify(payload),
      });
      console.log(`Call #${i}: Responded with HTTP Status -> ${response.status}`);
    } catch (error) {
      console.error(`Call #${i}: Failed with an error -> ${error.message}`);
    }
  }

  console.log("\n---\n🏁 Limit likely reached. Making the final call to see the error response...\n---\n");

  // 4. Make the 51st call to see the full error response.
  try {
    const finalResponse = await fetch(API_URL, {
      method: 'POST',
      headers: headers,
      body: JSON.stringify(payload),
    });

    console.log("--- Final Response ---");
    console.log(`HTTP Status: ${finalResponse.status}`); // Should be 429

    // Print headers
    console.log("\n--- Headers ---");
    for (const [key, value] of finalResponse.headers.entries()) {
      console.log(`${key}: ${value}`);
    }

    // Print the response body
    const responseBody = await finalResponse.json();
    console.log("\n--- Body ---");
    console.log(JSON.stringify(responseBody, null, 2)); // Pretty-print the JSON

  } catch (error) {
    console.error(`Final call failed with an error: ${error.message}`);
  }
}

// Run the main function.
testRateLimit();