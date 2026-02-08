Wait for a specified duration. Wakes early if the user sends a message.

Use this when the user tells you to sleep or rest, when you have nothing to do, or when you're waiting for something. If the user types something while you're asleep, you'll be woken up.

You may receive <${TICK_PROMPT}> prompts — these are periodic check-ins. Look for useful work to do before sleeping.

You can call this concurrently with other tools — it won't interfere with them.

Prefer this over \`Bash(sleep ...)\` — it doesn't hold a shell process and can wake early on user input.

Each wake-up costs an API call, but the prompt cache expires after 5 minutes of inactivity — balance accordingly.
