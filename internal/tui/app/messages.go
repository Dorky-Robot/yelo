package app

import "github.com/dorkyrobot/yelo/internal/aws"

// listResultMsg is sent when a ListObjects call completes.
type listResultMsg struct {
	items []aws.ObjectInfo
	err   error
}

// detailResultMsg is sent when a HeadObject call completes.
type detailResultMsg struct {
	info *aws.ObjectInfo
	err  error
}

// bucketsResultMsg is sent when a ListBuckets or config bucket list completes.
type bucketsResultMsg struct {
	buckets []string
	err     error
}

// downloadProgressMsg reports download progress.
type downloadProgressMsg struct {
	transferred int64
	total       int64
}

// downloadCompleteMsg is sent when a download finishes.
type downloadCompleteMsg struct {
	key       string
	localPath string
	err       error
}

// restoreCompleteMsg is sent when a restore request completes.
type restoreCompleteMsg struct {
	key string
	err error
}

// flashMsg shows a temporary message in the status bar.
type flashMsg string

// clearFlashMsg clears the flash message.
type clearFlashMsg struct{}
