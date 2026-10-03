package report

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

// Validator checks a letter against the facts it was written from.
type Validator struct {
	Map            *mapdata.Map
	MenPerStrength int
	GeneralNames   []string // every general's full name
	MinWords       int
	MaxWords       int
}

var digitsRe = regexp.MustCompile(`\d{1,3}(?:,\d{3})+|\d+`)

// Validate returns a list of violations; empty means the letter is fine.
//   - Every number in the text must match a number in the facts, or that
//     number times men-per-strength ("2,600 men" for strength 26). Number
//     words ("twelve hundred") are normalised first. Bare numbers up to ten
//     are allowed, since they are mostly days, riders and turns of phrase.
//   - Every province mentioned must appear in the facts, except the
//     sovereign's own capital, where the letter is addressed.
//   - No other general may be named unless the facts name him.
func (v Validator) Validate(text string, f ReportFacts) []string {
	var out []string
	allowed := map[int]bool{}
	for _, n := range append(f.Numbers(), NumbersIn(f.Text())...) {
		allowed[n] = true
		allowed[n*v.MenPerStrength] = true
	}
	for _, n := range NumbersIn(text) {
		if n > 10 && !allowed[n] {
			out = append(out, fmt.Sprintf("the number %d does not appear in FACTS", n))
		}
	}
	factText := f.Text()
	capital := v.Map.Capital(model.Player)
	for _, p := range v.Map.Provinces {
		if p.ID == capital || strings.Contains(factText, p.Name) {
			continue
		}
		for _, name := range append([]string{p.Name}, p.Aliases...) {
			if name == "" || name[0] < 'A' || name[0] > 'Z' {
				continue // lower-case aliases are common words
			}
			if containsWord(text, name) {
				out = append(out, fmt.Sprintf("%s is mentioned but is not in FACTS", p.Name))
				break
			}
		}
	}
	for _, g := range v.GeneralNames {
		if g == f.General || strings.Contains(factText, g) {
			continue
		}
		for _, part := range strings.Fields(g) {
			if len(part) > 3 && part != "the" && part != "Elder" && containsWord(text, part) {
				out = append(out, fmt.Sprintf("%s is mentioned but is not in FACTS", g))
				break
			}
		}
	}
	words := len(strings.Fields(text))
	if v.MinWords > 0 && words < v.MinWords {
		out = append(out, fmt.Sprintf("the letter has %d words; write at least %d", words, v.MinWords))
	}
	if v.MaxWords > 0 && words > v.MaxWords {
		out = append(out, fmt.Sprintf("the letter has %d words; write at most %d", words, v.MaxWords))
	}
	return out
}

func containsWord(text, word string) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	return re.MatchString(text)
}

var numberWords = map[string]int{
	"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7,
	"eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13,
	"fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18,
	"nineteen": 19, "twenty": 20, "thirty": 30, "forty": 40, "fifty": 50, "sixty": 60,
	"seventy": 70, "eighty": 80, "ninety": 90,
}

var wordRe = regexp.MustCompile(`[A-Za-z]+|\d{1,3}(?:,\d{3})+|\d+|[^\sA-Za-z\d]`)

// NumbersIn finds every number in a text, written as digits ("2,600") or
// words ("twelve hundred", "two thousand six hundred", "twenty-six").
func NumbersIn(text string) []int {
	var out []int
	tokens := wordRe.FindAllString(strings.ReplaceAll(text, "-", " "), -1)
	total, current, inRun := 0, 0, false
	flush := func() {
		if inRun {
			out = append(out, total+current)
		}
		total, current, inRun = 0, 0, false
	}
	for i, tok := range tokens {
		lower := strings.ToLower(tok)
		if digitsRe.MatchString(tok) && digitsRe.FindString(tok) == tok {
			flush()
			n, _ := strconv.Atoi(strings.ReplaceAll(tok, ",", ""))
			// "2 thousand", "12 hundred"
			if i+1 < len(tokens) {
				switch strings.ToLower(tokens[i+1]) {
				case "hundred":
					current, inRun = n, true
					continue
				case "thousand":
					current, inRun = n, true
					continue
				}
			}
			out = append(out, n)
			continue
		}
		if v, ok := numberWords[lower]; ok {
			current += v
			inRun = true
			continue
		}
		switch lower {
		case "hundred":
			if current == 0 {
				current = 1
			}
			current *= 100
			inRun = true
			continue
		case "thousand":
			if current == 0 {
				current = 1
			}
			total += current * 1000
			current = 0
			inRun = true
			continue
		case "and":
			// "two hundred and fifty" continues the run.
			if inRun && i+1 < len(tokens) {
				if _, ok := numberWords[strings.ToLower(tokens[i+1])]; ok {
					continue
				}
			}
		}
		flush()
	}
	flush()
	return out
}
