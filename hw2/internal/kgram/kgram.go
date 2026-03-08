// Пакет kgram реализует k-gram индекс для поиска по wildcard-паттернам.
//
// Алгоритм:
//  1. Каждый термин обрамляется маркером границы '$': "hello" → "$hello$"
//  2. Из обрамлённого термина извлекаются все k-граммы длины K (по умолчанию 3)
//  3. Для wildcard-запроса вида "he*lo" извлекаются k-граммы из префиксной ("$he")
//     и суффиксной ("lo$") частей, затем берётся пересечение термов из всех k-грамм
//  4. Результат фильтруется регулярным выражением от false positive
package kgram

import (
	"regexp"
	"sort"
	"strings"
)

const DefaultK = 3

type Index struct {
	k    int
	data map[string][]string // k-gram -> отсортированный список термов
}

func New(k int) *Index {
	return &Index{k: k, data: make(map[string][]string)}
}

// AddTerm добавляет термин в индекс
func (idx *Index) AddTerm(term string) {
	for _, kg := range kgrams(term, idx.k) {
		idx.data[kg] = append(idx.data[kg], term)
	}
}

// Lookup возвращает список термов, содержащих данный k-gram
func (idx *Index) Lookup(kg string) []string {
	return idx.data[kg]
}

func (idx *Index) ResolveWildcard(pattern string) []string {
	pattern = strings.ToLower(pattern)

	if !strings.Contains(pattern, "*") {
		if _, ok := idx.data[pattern]; ok {
			return []string{pattern}
		}
		return nil
	}

	required := kgramsForPattern(pattern, idx.k)

	if len(required) == 0 {
		// Паттерн слишком короткий для k-грамм — возвращаем все термы (дорого, но корректно)
		re := wildcardToRegexp(pattern)
		var result []string
		seen := make(map[string]struct{})
		for _, terms := range idx.data {
			for _, t := range terms {
				if _, dup := seen[t]; !dup && re.MatchString(t) {
					seen[t] = struct{}{}
					result = append(result, t)
				}
			}
		}
		sort.Strings(result)
		return result
	}

	// Берём постинг-лист для первого k-gram и пересекаем с остальными
	candidates := toSet(idx.data[required[0]])
	for _, kg := range required[1:] {
		posting := toSet(idx.data[kg])
		for term := range candidates {
			if _, ok := posting[term]; !ok {
				delete(candidates, term)
			}
		}
	}

	// Фильтруем false positive регулярным выражением
	re := wildcardToRegexp(pattern)
	result := make([]string, 0, len(candidates))
	for term := range candidates {
		if re.MatchString(term) {
			result = append(result, term)
		}
	}
	sort.Strings(result)
	return result
}

// kgrams возвращает все k-граммы термина с $ маркерами
func kgrams(term string, k int) []string {
	padded := "$" + term + "$"
	runes := []rune(padded)
	if len(runes) < k {
		return []string{padded}
	}
	result := make([]string, 0, len(runes)-k+1)
	for i := 0; i <= len(runes)-k; i++ {
		result = append(result, string(runes[i:i+k]))
	}
	return result
}

// kgramsForPattern извлекает k-граммы из частей паттерна по обе стороны от '*'
func kgramsForPattern(pattern string, k int) []string {
	parts := strings.SplitN(pattern, "*", 2)
	prefix := parts[0]
	suffix := ""
	if len(parts) > 1 {
		suffix = pattern[strings.LastIndex(pattern, "*")+1:]
	}

	seen := make(map[string]struct{})
	var result []string

	addKgrams := func(s string) {
		runes := []rune(s)
		for i := 0; i <= len(runes)-k; i++ {
			kg := string(runes[i : i+k])
			if _, dup := seen[kg]; !dup {
				seen[kg] = struct{}{}
				result = append(result, kg)
			}
		}
	}

	if len(prefix) > 0 {
		addKgrams("$" + prefix)
	}
	if len(suffix) > 0 {
		addKgrams(suffix + "$")
	}

	return result
}

// wildcardToRegexp преобразует wildcard-паттерн в регулярное выражение
func wildcardToRegexp(pattern string) *regexp.Regexp {
	var sb strings.Builder
	sb.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			sb.WriteString(".*")
		case '.', '+', '?', '(', ')', '[', ']', '{', '}', '\\', '^', '$', '|':
			sb.WriteRune('\\')
			sb.WriteRune(r)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteString("$")
	re, _ := regexp.Compile(sb.String())
	return re
}

func toSet(terms []string) map[string]struct{} {
	s := make(map[string]struct{}, len(terms))
	for _, t := range terms {
		s[t] = struct{}{}
	}
	return s
}
