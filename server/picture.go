package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"
)

// The plugin API can upload a profile picture but can't reset one to the
// server's generated default, so a removed picture is replaced with this
// neutral silhouette instead.
var (
	placeholderPictureOnce sync.Once
	placeholderPicture     []byte
	placeholderPictureErr  error
)

const placeholderPictureSize = 256

func bannedPlaceholderPicture() ([]byte, error) {
	placeholderPictureOnce.Do(func() {
		placeholderPicture, placeholderPictureErr = renderPlaceholderPicture(placeholderPictureSize)
	})
	return placeholderPicture, placeholderPictureErr
}

// renderPlaceholderPicture draws a white head-and-shoulders silhouette on a
// neutral gray background.
func renderPlaceholderPicture(size int) ([]byte, error) {
	background := color.RGBA{R: 0x8d, G: 0x93, B: 0xa5, A: 0xff}
	foreground := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	s := float64(size)
	headX, headY, headR := s/2, s*0.39, s*0.18
	bodyX, bodyY, bodyRX, bodyRY := s/2, s*1.02, s*0.36, s*0.4

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			px, py := float64(x)+0.5, float64(y)+0.5
			dx, dy := px-headX, py-headY
			inHead := dx*dx+dy*dy <= headR*headR
			ex, ey := (px-bodyX)/bodyRX, (py-bodyY)/bodyRY
			inBody := ex*ex+ey*ey <= 1
			if inHead || inBody {
				img.Set(x, y, foreground)
			} else {
				img.Set(x, y, background)
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// removeProfilePicture replaces a member's profile picture with the
// placeholder.
func (p *Plugin) removeProfilePicture(userID string) error {
	picture, err := bannedPlaceholderPicture()
	if err != nil {
		return err
	}
	return p.client.User.SetProfileImage(userID, bytes.NewReader(picture))
}
