## Запуск

```bash
# Первый запуск — проиндексирует docs/ и запустит интерактивный поиск
go run .

# Пересобрать индекс с нуля
go run . -reindex

# Режим без стемминга (нужен для prefix/wildcard поиска)
go run . -nostem -reindex

# Дополнительные флаги
go run . -docs ./docs -data ./data -lang english -flush 200
```

| Флаг | По умолчанию | Описание |
|---|---|---|
| `-docs` | `docs` | Папка с `.txt` документами |
| `-data` | `data` | Папка для хранения индекса на диске |
| `-lang` | `english` | Язык стемминга: `english` или `russian` |
| `-flush` | `200` | Сброс MemTable после N пар (term, docID) |
| `-reindex` | `false` | Удалить старый индекс и пересобрать |
| `-nostem` | `false` | Отключить стемминг и стоп-слова |

## Примеры запросов

### Булев поиск

```
# Неявный AND — два слова рядом
> fox hound
Found 1 document(s)  [query: fox hound]
  [  0] doc01.txt   The Red Fox and the Hound ...

# Явный AND
> machine AND learning
Found 1 document(s)  [query: machine AND learning]
  [  1] doc02.txt   Introduction to Machine Learning ...

# OR
> fox OR climate
Found 2 document(s)  [query: fox OR climate]
  [  0] doc01.txt   The Red Fox and the Hound ...
  [  5] doc06.txt   Climate Change and Global Warming ...

# NOT
> database NOT sql
Found 1 document(s)  [query: database NOT sql]
  [  2] doc03.txt   Database Systems and Indexing ...

# Скобки и смешанные операторы
> (fox OR hound) AND NOT climate
Found 1 document(s)  [query: (fox OR hound) AND NOT climate]
  [  0] doc01.txt   The Red Fox and the Hound ...

# Термины стеммируются автоматически: "running" → "run", "computers" → "comput"
> computing
Found 2 document(s)  [query: computing]
  ...
```

### Поиск по префиксу (рекомендуется `-nostem`)

Запуск: `go run . -nostem -reindex`

```
# Все слова начинающиеся с "comput"
> comput*
Found 3 document(s)  [query: comput*]
  [  1] doc02.txt   Introduction to Machine Learning ...
  [  4] doc05.txt   Programming Languages Overview ...
  [  7] doc08.txt   The Internet and Computer Networks ...

# Префикс "inter"
> inter*
Found 2 document(s)  [query: inter*]
  [  1] doc02.txt   ...
  [  7] doc08.txt   The Internet and Computer Networks ...

# Можно комбинировать с булевыми операторами
> comput* AND NOT network*
Found 1 document(s)  [query: comput* AND NOT network*]
  [  4] doc05.txt   Programming Languages Overview ...
```

### Wildcard поиск через k-gram индекс (рекомендуется `-nostem`)

Запуск: `go run . -nostem -reindex`

```
# Звёздочка в середине — k-gram поиск
> br*wn
Found 1 document(s)  [query: br*wn]
  [  0] doc01.txt   The Red Fox and the Hound ...   # "brown"

# Суффикс — ведущая звёздочка
> *tion
Found 7 document(s)  [query: *tion]
  ...   # "introduction", "information", "population", ...

# Паттерн с несколькими символами
> c*t
Found 4 document(s)  [query: c*t]
  ...   # "cat", "coast", "connect", "consist", ...

# Комбинация с булевым
> *tion AND NOT *ment
Found 5 document(s)  [query: *tion AND NOT *ment]
  ...
```

