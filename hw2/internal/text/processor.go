package text

import (
	"strings"
	"unicode"

	"github.com/kljensen/snowball"
)

type Processor struct {
	stopWords map[string]struct{}
	language  string
	NoStem    bool // если true — пропускаем стемминг и стоп-слова (нужно для prefix/wildcard поиска)
}

func NewProcessor(language string) *Processor {
	p := &Processor{
		stopWords: make(map[string]struct{}),
		language:  language,
	}
	for _, w := range StopWords(language) {
		p.stopWords[w] = struct{}{}
	}
	return p
}

// токенизирует текст документа
func (p *Processor) Process(text string) []string {
	tokens := tokenize(text)
	result := make([]string, 0, len(tokens))

	for _, token := range tokens {
		token = strings.ToLower(token)
		if len(token) < 2 { // одиночные символы вроде "s" или "a" не индексируем
			continue
		}
		if !p.NoStem {
			if _, isStop := p.stopWords[token]; isStop {
				continue
			}
		}
		var term string
		if p.NoStem {
			term = token
		} else {
			term = p.stem(token)
		}
		if len(term) > 0 {
			result = append(result, term)
		}
	}
	return result
}

// токенизирует одно слово для поиска
func (p *Processor) ProcessTerm(term string) string {
	term = strings.ToLower(strings.TrimSpace(term))
	if len(term) < 2 {
		return ""
	}
	if p.NoStem {
		return term
	}
	if _, isStop := p.stopWords[term]; isStop {
		return ""
	}
	return p.stem(term)
}

func (p *Processor) stem(word string) string {
	stemmed, err := snowball.Stem(word, p.language, true)
	if err != nil || len(stemmed) == 0 {
		return word
	}
	return stemmed
}

func tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
