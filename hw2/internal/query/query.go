// Грамматика булевых запросов (приоритет: NOT > AND > OR):
//
//	or_expr  = and_expr  ( "OR"  and_expr  )*
//	and_expr = not_expr  ( "AND"? not_expr )*   // два слова рядом — неявный AND
//	not_expr = "NOT" not_expr | primary
//	primary  = WORD | "(" or_expr ")"
//
// WORD может содержать '*' — тогда применяется prefix или wildcard поиск.
package query

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/RoaringBitmap/roaring"

	"hw2/internal/kgram"
	"hw2/internal/lsm"
)

const dateFmt = "2006-01-02"

type tokenKind int

const (
	tokWord tokenKind = iota
	tokAND
	tokOR
	tokNOT
	tokLParen
	tokRParen
	tokEOF
	tokValidRange    // VALID:[from,to]
	tokAppearedRange // APPEARED:[from,to]
)

// parseDateRange разбирает строку вида "2020-01-01,2021-12-31".
func parseDateRange(s string) (from, to time.Time, err error) {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return from, to, fmt.Errorf("date range must be 'from,to', got %q", s)
	}
	if parts[0] == "" {
		from = time.Unix(0, 0).UTC()
	} else {
		from, err = time.Parse(dateFmt, parts[0])
		if err != nil {
			return from, to, fmt.Errorf("invalid from date %q: %w", parts[0], err)
		}
	}
	if parts[1] == "" {
		to = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	} else {
		to, err = time.Parse(dateFmt, parts[1])
		if err != nil {
			return from, to, fmt.Errorf("invalid to date %q: %w", parts[1], err)
		}
	}
	return from, to, nil
}

type token struct {
	kind tokenKind
	val  string
}

func lex(q string) []token {
	var tokens []token
	i, n := 0, len(q)
	for i < n {
		for i < n && unicode.IsSpace(rune(q[i])) {
			i++
		}
		if i >= n {
			break
		}
		switch q[i] {
		case '(':
			tokens = append(tokens, token{tokLParen, "("})
			i++
		case ')':
			tokens = append(tokens, token{tokRParen, ")"})
			i++
		default:
			j := i
			for j < n && q[j] != '(' && q[j] != ')' && !unicode.IsSpace(rune(q[j])) {
				j++
			}
			word := q[i:j]
			i = j
			upper := strings.ToUpper(word)
			switch {
			case upper == "AND":
				tokens = append(tokens, token{tokAND, "AND"})
			case upper == "OR":
				tokens = append(tokens, token{tokOR, "OR"})
			case upper == "NOT":
				tokens = append(tokens, token{tokNOT, "NOT"})
			case strings.HasPrefix(upper, "VALID:[") && strings.HasSuffix(word, "]"):
				inner := word[len("VALID:[") : len(word)-1]
				tokens = append(tokens, token{tokValidRange, inner})
			case strings.HasPrefix(upper, "APPEARED:[") && strings.HasSuffix(word, "]"):
				inner := word[len("APPEARED:[") : len(word)-1]
				tokens = append(tokens, token{tokAppearedRange, inner})
			default:
				tokens = append(tokens, token{tokWord, word})
			}
		}
	}
	tokens = append(tokens, token{tokEOF, ""})
	return tokens
}

type parser struct {
	tokens []token
	pos    int
	idx    *lsm.LSM
	kg     *kgram.Index
}

func (p *parser) peek() token { return p.tokens[p.pos] }
func (p *parser) consume() token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

// Evaluate разбирает запрос и сразу возвращает bitmap подходящих документов
func Evaluate(q string, idx *lsm.LSM, kg *kgram.Index) (*roaring.Bitmap, error) {
	tokens := lex(strings.TrimSpace(q))
	if tokens[0].kind == tokEOF {
		return nil, fmt.Errorf("empty query")
	}
	p := &parser{tokens: tokens, idx: idx, kg: kg}
	result, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("unexpected token %q", p.peek().val)
	}
	return result, nil
}

func (p *parser) parseOr() (*roaring.Bitmap, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOR {
		p.consume()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = roaring.Or(left, right)
	}
	return left, nil
}

func (p *parser) parseAnd() (*roaring.Bitmap, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		k := p.peek().kind
		if k == tokAND {
			p.consume() // явный AND
		} else if k == tokWord || k == tokLParen || k == tokNOT ||
			k == tokValidRange || k == tokAppearedRange {
			// неявный AND: два токена рядом без оператора
		} else {
			break
		}
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = roaring.And(left, right)
	}
	return left, nil
}

func (p *parser) parseNot() (*roaring.Bitmap, error) {
	if p.peek().kind == tokNOT {
		p.consume()
		expr, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		universe := p.idx.Universe()
		universe.AndNot(expr)
		return universe, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (*roaring.Bitmap, error) {
	t := p.peek()
	switch t.kind {
	case tokWord:
		p.consume()
		return p.lookupTerm(t.val), nil
	case tokValidRange:
		p.consume()
		from, to, err := parseDateRange(t.val)
		if err != nil {
			return nil, err
		}
		return p.idx.ValidInRange(from, to), nil
	case tokAppearedRange:
		p.consume()
		from, to, err := parseDateRange(t.val)
		if err != nil {
			return nil, err
		}
		return p.idx.AppearedInRange(from, to), nil
	case tokLParen:
		p.consume()
		result, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf("expected ')' but got %q", p.peek().val)
		}
		p.consume()
		return result, nil
	default:
		return nil, fmt.Errorf("expected term or '(' but got %q", t.val)
	}
}

// lookupTerm определяет тип запроса и вызывает нужный метод поиска
func (p *parser) lookupTerm(term string) *roaring.Bitmap {
	if !strings.Contains(term, "*") {
		return p.idx.Lookup(term)
	}

	term = strings.ToLower(term)

	// Поиск по префиксу: "word*" — звёздочка только в конце
	if strings.HasSuffix(term, "*") && !strings.Contains(term[:len(term)-1], "*") {
		return p.idx.PrefixLookup(term[:len(term)-1])
	}

	// Wildcard-поиск через k-gram индекс
	if p.kg != nil {
		if matching := p.kg.ResolveWildcard(term); len(matching) > 0 {
			return p.idx.WildcardLookup(matching)
		}
	}
	return roaring.New()
}
