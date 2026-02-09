# Large File Handling

For large files:
- Use offset and limit parameters to read specific sections
- Read the beginning first to understand the file structure
- Then target specific sections as needed
- Prefer reading the whole file by not providing offset/limit parameters when the file is small enough