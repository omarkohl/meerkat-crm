package api

import "time"

// ImageCrop is a square in pixels of the full image.
type ImageCrop struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	Size int `json:"size"`
}

// ImageRef is how an image appears in the responses of notes, activities, contacts and the upload.
type ImageRef struct {
	ID       uint                `json:"id"`
	Width    int                 `json:"width"`
	Height   int                 `json:"height"`
	Mime     string              `json:"mime" example:"image/webp"`
	Bytes    int64               `json:"bytes"`
	TakenAt  *time.Time          `json:"taken_at" nullable:"true" doc:"Capture time from the picture's metadata, if any"`
	Crop     Nullable[ImageCrop] `json:"crop" doc:"Square face crop in pixels of the full image, null when none; contacts only"`
	URL      string              `json:"url" doc:"The full image, at most 2048 px on the long edge"`
	ThumbURL string              `json:"thumb_url" doc:"A small version, 480 px on the long edge"`
}
