# 🐾 Twitch Quote Bot (Go)

**by [ItsNickDoberman](https://twitch.tv/ItsNickDoberman)**  
A simple, lightweight, cross-platform Twitch quote system written in Go — built from scratch *in Termux on a phone during vacation*.  
Yes. Really.

---

## 🌟 What It Does

This bot lets you manage Twitch chat quotes via IRC using easy `!quote` commands — add, search, delete, and list quotes.  
It stores them all in a local SQLite database and supports both **Twitch mode** and **CLI mode** for quick testing.

It's fast, tiny, and OS-agnostic — works on Linux, Windows, macOS, or even Raspberry Pi.

---

## ⚙️ Features

- 🎤 `!quote add <text>` — Add a new quote
- ❓ `!quote search <term>` — Find a quote by text or author
- 🎲 `!quote` — Grab a random quote
- 📜 `!quote list` — List the first 5 quotes
- ❌ `!quote delete <id>` — Remove a quote (mod-only suggested)
- 🛠️ CLI Mode — Add/search/list/delete quotes manually in terminal
- 🐧 Runs anywhere Go runs (no external dependencies besides SQLite)

---

## Installation

### 1. Clone the repo

```bash
git clone https://github.com/yourusername/twitch-quote-bot.git
cd twitch-quote-bot
````

### 2. Build it

```bash
go build -o twitchquote .
```

You’ll get a `twitchquote` binary you can run directly.

---

## Usage

### Twitch Mode (Connects to chat)

```bash
./twitchquote -mode twitch -user your_bot_username -oauth oauth:yourtoken -channel yourchannel
```

> You’ll need a [Twitch IRC token](https://twitchapps.com/tmi/) for this to work.

---

### CLI Mode (Local DB testing)

```bash
./twitchquote -mode cli
```

You’ll get a prompt for adding, listing, or searching quotes.

---

## ❗ Note from Nick

Hey there! I'm Nick — a dobie V-Tuber and rookie coder learning Go and building tools for my own chaos-fueled stream.

This is a **work in progress**, and may not always behave perfectly.
If you find bugs, or want to improve things — **pull requests are absolutely welcome!**

---

## 🐾 License

GPL — go wild, keep it open source and credit me if you're using it pubicly 

---

## 💬 Contact / Credit

* Twitch: [ItsNickDoberman](https://twitch.tv/ItsNickDoberman)
* GitHub: [github.com/sillynickdev](https://github.com/sillynickdev5)
* Bluesky: [@nickdoberman.xyz](https://bsky.app/profile/nickdoberman.xyz)

---

> Bark. Chaos. Repeat.


