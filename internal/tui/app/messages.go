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

// bucketsResultMsg is sent when a ListBuckets call completes.
type bucketsResultMsg struct {
	buckets []string
	err     error
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

// profilesResultMsg is sent when AWS profiles are loaded.
type profilesResultMsg struct {
	profiles []string
	err      error
}

// profileTestMsg is sent when a profile connectivity test completes.
type profileTestMsg struct {
	profile string
	ok      bool
	err     error
}

// clearFlashMsg clears the status message after a delay.
type clearFlashMsg struct{}
