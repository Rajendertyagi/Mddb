package main

import (
	"regexp"
	"strings"
	"unicode"
)

// Pre-compiled regexes for markdown stripping.
var (
	reCodeBlock = regexp.MustCompile("(?s)```.*?```")
	reHTMLTag   = regexp.MustCompile(`<[^>]+>`)
	reImgLink   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reLink      = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	reHeader    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
)

// positiveWords is a lexicon of positive English words.
var positiveWords = map[string]bool{
	"good": true, "great": true, "excellent": true, "amazing": true, "wonderful": true,
	"fantastic": true, "awesome": true, "outstanding": true, "superb": true, "brilliant": true,
	"perfect": true, "love": true, "loved": true, "loving": true, "lovely": true,
	"beautiful": true, "happy": true, "happiness": true, "joy": true, "joyful": true,
	"delightful": true, "pleasant": true, "cheerful": true, "excited": true, "exciting": true,
	"thrilled": true, "grateful": true, "thankful": true, "blessed": true, "impressive": true,
	"remarkable": true, "magnificent": true, "splendid": true, "terrific": true, "marvelous": true,
	"incredible": true, "exceptional": true, "fabulous": true, "glorious": true, "elegant": true,
	"graceful": true, "charming": true, "admirable": true, "inspiring": true, "innovative": true,
	"successful": true, "triumph": true, "victory": true, "win": true, "winning": true,
	"positive": true, "optimistic": true, "hopeful": true, "bright": true, "radiant": true,
	"vibrant": true, "enthusiastic": true, "passionate": true, "creative": true, "genius": true,
	"generous": true, "kind": true, "friendly": true, "helpful": true, "supportive": true,
	"reliable": true, "trustworthy": true, "honest": true, "brave": true, "courageous": true,
	"strong": true, "powerful": true, "efficient": true, "effective": true, "productive": true,
	"valuable": true, "worthy": true, "superior": true, "premium": true, "flawless": true,
	"recommend": true, "recommended": true, "praise": true, "praised": true, "celebrate": true,
	"enjoy": true, "enjoyed": true, "enjoyable": true, "fun": true, "pleasure": true,
	"satisfied": true, "satisfying": true, "smooth": true, "clean": true, "comfortable": true,
	"safe": true, "secure": true, "stable": true, "solid": true, "robust": true,
}

// negativeWords is a lexicon of negative English words.
var negativeWords = map[string]bool{
	"bad": true, "terrible": true, "horrible": true, "awful": true, "dreadful": true,
	"hate": true, "hated": true, "hating": true, "hateful": true, "disgusting": true,
	"ugly": true, "nasty": true, "worst": true, "worse": true, "poor": true,
	"pathetic": true, "worthless": true, "useless": true, "broken": true, "failed": true,
	"failure": true, "disappointing": true, "disappointed": true, "frustrating": true, "frustrated": true,
	"annoying": true, "annoyed": true, "angry": true, "furious": true, "rage": true,
	"tragic": true, "devastating": true, "catastrophic": true, "disastrous": true, "ruined": true,
	"destroyed": true, "abysmal": true, "atrocious": true, "deplorable": true, "miserable": true,
	"painful": true, "suffering": true, "agony": true, "torment": true, "nightmare": true,
	"boring": true, "dull": true, "tedious": true, "mediocre": true, "inferior": true,
	"weak": true, "flawed": true, "defective": true, "faulty": true, "buggy": true,
	"slow": true, "clumsy": true, "awkward": true, "confusing": true, "complicated": true,
	"unreliable": true, "unstable": true, "insecure": true, "dangerous": true, "harmful": true,
	"toxic": true, "corrupt": true, "dishonest": true, "deceitful": true, "cruel": true,
	"hostile": true, "aggressive": true, "violent": true, "rude": true, "offensive": true,
	"shameful": true, "embarrassing": true, "humiliating": true, "degrading": true, "insulting": true,
	"reject": true, "rejected": true, "abandon": true, "abandoned": true, "neglect": true,
	"ignore": true, "ignored": true, "waste": true, "wasted": true, "loss": true,
	"regret": true, "sorry": true, "sad": true, "depressing": true, "depressed": true,
	"anxious": true, "worried": true, "scared": true, "fear": true, "fearful": true,
}

// stripMarkdown removes common markdown syntax from text, preserving content words.
func stripMarkdown(text string) string {
	// Remove code blocks first (they may contain false positives)
	text = reCodeBlock.ReplaceAllString(text, " ")
	// Remove images
	text = reImgLink.ReplaceAllString(text, " ")
	// Replace links with their text
	text = reLink.ReplaceAllString(text, "$1")
	// Remove HTML tags
	text = reHTMLTag.ReplaceAllString(text, " ")
	// Remove header markers
	text = reHeader.ReplaceAllString(text, "")
	// Remove bold/italic markers
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "__", "")
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "_", " ")
	// Remove inline code backticks
	text = strings.ReplaceAll(text, "`", "")
	return text
}

// AnalyzeSentiment returns a sentiment score from -1.0 (very negative) to +1.0 (very positive).
// Uses a simple keyword/lexicon-based approach. Returns 0.0 for empty text.
func AnalyzeSentiment(text string) float64 {
	if text == "" {
		return 0.0
	}

	text = stripMarkdown(text)
	text = strings.ToLower(text)

	var pos, neg int
	var word strings.Builder

	processWord := func() {
		if word.Len() >= 2 {
			w := word.String()
			if positiveWords[w] {
				pos++
			} else if negativeWords[w] {
				neg++
			}
		}
		word.Reset()
	}

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(r)
		} else {
			processWord()
		}
	}
	processWord() // last word

	total := pos + neg
	if total == 0 {
		return 0.0
	}
	return float64(pos-neg) / float64(total)
}
