# Go Todo CLI

A minimalist, tree-based Todo list application running in your terminal. Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).



## Features

* 🌳 **Tree Structure**: Nested tasks with infinite depth.
* 🗑️ **Recursive Delete**: Deleting a parent task automatically moves its children to the trash.
* ♻️ **Persistent Trash**: Deleted items are stored in the file (tagged `[D]`) and can be restored even after restart.
* 🎨 **Themes**: Switch between Gruvbox, Dracula, and Monokai (loaded from `themes.json`).
* 💾 **Persistence**: Auto-saves to `todo.md` and remembers your theme preference in `config.json`.

## Installation

### Method 1: Go Install (Easiest)

If you have Go installed, you can install the application directly without cloning the repository:

```bash
go install github.com/pawello85/todo@latest
```
### Method 2: from source

```bash 
git clone https://github.com/pawello85/todo.git
cd todo
go install
```