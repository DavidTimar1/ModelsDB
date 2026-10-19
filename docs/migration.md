# Migrating an old database

## Contents

- [What happened](#what-happened)
- [Do this first](#do-this-first)
- [Find out what version you have](#find-out-what-version-you-have)
- [Carry the database forward](#carry-the-database-forward)
- [What changes](#what-changes)
- [If you get stuck](#if-you-get-stuck)

## What happened

Your `modelsdb.db` was made by an older ModelsDB, one that stored its data in a shape this version no longer upgrades. This version refused to open it, and changed nothing in it, rather than guess at what it holds. One conversion version carries such a database forward in a single step, and this page says how to use it.

This version is not backwards compatible with ModelsDB 1.29.0 and 1.30.1: there is no automatic upgrade from them, and no way back to them except the copy you make below. The steps need a terminal. We recommend pasting this page into an AI assistant (Claude, ChatGPT or similar) and letting it guide you through each step on your own system.

## Do this first

Stop ModelsDB. Copy `modelsdb.db`, `modelsdb.db-wal` and `modelsdb.db-shm` together into a folder outside your data folder. All three files, and only while ModelsDB is not running. Everything below is safe only because this copy exists.

## Find out what version you have

Install any SQLite client and run:

```sql
PRAGMA user_version;
```

A number below 33 means this page is for you: a database from ModelsDB 1.29.0 answers 3, one from 1.30.1 answers 5. Do not read the file with a hex editor or a "SQLite header viewer" to find this out. SQLite writes the current version into the write-ahead log while the log is unread, so the number sitting in the file's own header can lag behind the truth. Running `PRAGMA user_version` through SQLite itself is the only reading that is honest.

## Carry the database forward

The conversion version is a pre-release, kept apart from the normal releases so that no installed copy of ModelsDB ever offers it as an update: <https://github.com/DavidTimar1/ModelsDB/releases/tag/bridge-8.11.4>

It carries every database from ModelsDB 1.29.0 and 1.30.1 up to the point this version takes over. It is not signed. Check the file you download against `SHA256SUMS` from the same release page.

1. Stop ModelsDB, and make the copy described above.
2. Download the file for your system from the release page: `modelsdb-bridge-linux-amd64` or `modelsdb-bridge-windows-amd64.exe`. On Linux, make it executable with `chmod +x modelsdb-bridge-linux-amd64`.
3. Run its `backup` command once, against the folder that holds your `modelsdb.db`, with any existing folder to receive a backup file (create it first). It converts the database, needs no network and starts no server:

   ```bash
   ./modelsdb-bridge-linux-amd64 --data-dir /path/to/your/data backup /path/to/a/backup/folder
   ```

   The conversion version does not check whether ModelsDB is running, so make sure it is stopped.
4. Start this version of ModelsDB against the same data folder. It opens the converted database and carries it the rest of the way by itself.

## What changes

- Everything you typed is kept: notes (any pricing note is kept in the note), favourites, your own columns and their values.
- The old HuggingFace architecture and weight-format columns are written into each model's Architecture line before they are deleted; nothing they measured is lost.
- The view settings (column order, widths, sort) of the old version are not carried over. You set them again.
- Cells of columns this version no longer has are dropped. Values you typed are not among them.
- A figure you typed into one of the old fixed fields (parameters, MoE, tool) comes back under that field's fetched source, because the old file cannot tell your figure from the shipped catalog's.
- A column of your own whose name matches a shipped column, such as "Parameters (B)", and whose values the shipped column cannot take, is kept as a separate column with " (mine)" added to its label.
- The old file `curated.json`, if you have one, is left where it is and is no longer read.

## If you get stuck

Restore the three files from your copy, and tell me through one of the channels on my [GitHub profile](https://github.com/DavidTimar1), with the output of `PRAGMA user_version` and your model count. Do not send the database itself, it holds your notes.
