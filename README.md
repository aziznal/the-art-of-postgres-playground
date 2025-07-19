# The art of postgres - my playground

This is a bunch of stuff I tried while reading the art of postgres. To make
sense of this, you must have the extras provided when you purchase the book.

If you are me in the future, check your email for the course details. If you're
someone else, [buy the book here](https://theartofpostgresql.com/).

## Docker command

```bash
docker run --name pg -e POSTGRES_PASSWORD=pass -p 5432:5432 -d postgres
```

## Connection URL

```
postgresql://postgres:pass@localhost:5432/postgres
```

## pgloader

[Github link](https://github.com/dimitri/pgloader)

Command to load a dataset:

```
pgloader Chinook_Sqlite_AutoIncrementPKs.sqlite postgresql://postgres:pass@localhost:5432/postgres
```
