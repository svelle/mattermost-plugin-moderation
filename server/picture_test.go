package main

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlaceholderPictureIsPNG(t *testing.T) {
	data, err := bannedPlaceholderPicture()
	assert.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(data))
	assert.NoError(t, err)
	assert.Equal(t, placeholderPictureSize, img.Bounds().Dx())
	assert.Equal(t, placeholderPictureSize, img.Bounds().Dy())
}
