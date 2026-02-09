You are capturing this session's repeatable process as a reusable skill.

## Your Task

### Step 1: Analyze the Session

Before asking any questions, analyze the session to identify:
- What repeatable process was performed
- What the inputs/parameters were
- The distinct steps (in order)
- The success artifacts/criteria for each step
- Where the user corrected or steered you
- What tools and permissions were needed
- What agents were used
- What the goals and success artifacts were

### Step 2: Interview the User

Use AskUserQuestion to understand what the user wants to automate:
- Use AskUserQuestion for ALL questions! Never ask questions via plain text.
- For each round, iterate as much as needed until the user is happy.

**Round 1: High level confirmation**
- Suggest a name and description for the skill based on your analysis.
- Suggest high-level goal(s) and specific success criteria.

**Round 2: More details**
- Present the high-level steps you identified as a numbered list.
- Suggest arguments based on what you observed.
- Ask if this skill should run inline or forked.

**Round 3: Breaking down each step**
For each major step, ask about success criteria, dependencies, human checkpoints, and constraints.

**Round 4: Final questions**
- Confirm when this skill should be invoked, and suggest trigger phrases.

### Step 3: Write the SKILL.md

Create the skill at `.claude/skills/{skill-name}/SKILL.md` with YAML frontmatter including:
- name, description, allowed-tools, when_to_use, argument-hint, arguments, context
- Body with: Inputs, Goal, Steps (each with success criteria)

### Step 4: Confirm and Save

Show the user the complete SKILL.md and ask for final confirmation.
After writing, tell them how to invoke it: `/{skill-name} [arguments]`.
