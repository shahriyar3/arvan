package domain

import (
	"regexp"
	"unicode/utf8"

	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
)

var e164Pattern = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

const (
	GSM7MaxLength = 160
	UCS2MaxLength = 70
)

// gsm7BasicChars is the GSM 03.38 default alphabet (single-page detection).
const gsm7BasicChars = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞ ÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"

var gsm7Allowed = func() map[rune]struct{} {
	set := make(map[rune]struct{}, utf8.RuneCountInString(gsm7BasicChars))
	for _, r := range gsm7BasicChars {
		set[r] = struct{}{}
	}
	return set
}()

func ValidateE164(number string) bool {
	return e164Pattern.MatchString(number)
}

func DetectEncoding(body string) string {
	for _, r := range body {
		if _, ok := gsm7Allowed[r]; !ok {
			return EncodingUCS2
		}
	}
	return EncodingGSM7
}

func MaxLengthForEncoding(encoding string) int {
	if encoding == EncodingUCS2 {
		return UCS2MaxLength
	}
	return GSM7MaxLength
}

func ValidateSinglePageBody(body string) (encoding string, err error) {
	encoding = DetectEncoding(body)
	length := utf8.RuneCountInString(body)
	if length > MaxLengthForEncoding(encoding) {
		return encoding, domainerrors.ErrMessageTooLong
	}
	return encoding, nil
}
