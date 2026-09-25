package lan

import (
	"github.com/dawsonxiong/thock/internal/content"
	"github.com/dawsonxiong/thock/internal/race"
)

// Text draws a round's text from thock's own word lists and quotes.
func Text(s race.Setup) ([]string, string, error) {
	if s.Mode == race.ModeQuotes {
		q, err := content.RandomQuote(content.Length(s.Length))
		if err != nil {
			return nil, "", err
		}
		return content.QuoteWords(q), q.Attribution(), nil
	}
	words, err := content.Words(content.List(s.List), s.Words)
	return words, "", err
}
