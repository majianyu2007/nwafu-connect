package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/majianyu2007/nwafu-connect/client/authchallenge"
)

type graphCheckCodePayload struct {
	Coordinates [][]int `json:"coordinates"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
}

func graphCheckCodeFromResponse(response authchallenge.ClickCaptchaResponse, imgData []byte) (string, error) {
	if len(response.Points) == 0 {
		return "", fmt.Errorf("graph check code has no click points")
	}

	width, height := response.Width, response.Height
	if width <= 0 || height <= 0 {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(imgData))
		if err != nil {
			return "", fmt.Errorf("missing captcha image dimensions and failed to decode image: %w", err)
		}
		width, height = cfg.Width, cfg.Height
	}

	coordinates := make([][]int, 0, len(response.Points))
	for _, point := range response.Points {
		coordinates = append(coordinates, []int{point.X, point.Y})
	}

	payload := graphCheckCodePayload{
		Coordinates: coordinates,
		Width:       width,
		Height:      height,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func canonicalizeGraphCheckCode(raw string, imgData []byte) (string, error) {
	response, err := authchallenge.ParseClickCaptchaResponse(raw)
	if err != nil {
		return "", err
	}
	return graphCheckCodeFromResponse(response, imgData)
}
