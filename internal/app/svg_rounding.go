package app

import "icon-svg/internal/svgstyle"

func prepareUploadedSVG(content []byte) ([]byte, error) {
	validated, err := validateSVG(content)
	if err != nil {
		return nil, err
	}
	rounded, _, err := svgstyle.RoundSquare(validated)
	if err != nil {
		return nil, err
	}
	return validateSVG(rounded)
}
