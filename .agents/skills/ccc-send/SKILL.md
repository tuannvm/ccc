---
name: ccc-send
description: Send generated or built files to the user via Telegram using `ccc send`.
allowed-tools: Bash
---

# CCC Send - File Transfer

## Description
Send files to the user via Telegram using the `ccc send` command.

## Usage
When the user asks you to send them a file, or when you have generated or built a file that the user needs, use:

```bash
ccc send <file_path>
```

## How It Works
- Small files under 50 MB are sent directly via Telegram.
- Large files are streamed via the CCC relay server with a one-time download link.

## Examples

```bash
ccc send ./build/app.apk
ccc send ./output/report.pdf
ccc send ~/Downloads/large-file.zip
```

## Important Notes
- The command detects the current session from your working directory.
- For large files, the command waits up to 10 minutes for the user to download.
- Each download link is one-time use only.
- Use this proactively when you have created files the user needs.
