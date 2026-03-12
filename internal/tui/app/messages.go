package app

import "github.com/dorkyrobot/yelo/internal/aws"

type listResultMsg struct {
	items []aws.ObjectInfo
	err   error
}

type detailResultMsg struct {
	info *aws.ObjectInfo
	err  error
}

type bucketsResultMsg struct {
	buckets []string
	err     error
}

type downloadCompleteMsg struct {
	key       string
	localPath string
	err       error
}

type restoreCompleteMsg struct {
	key string
	err error
}

type profilesResultMsg struct {
	profiles []string
	err      error
}

type profileTestMsg struct {
	profile string
	bucket  string
	ok      bool
	err     error
}

// awsConfigDoneMsg is sent after `aws configure` finishes.
type awsConfigDoneMsg struct {
	err error
}

type clearFlashMsg struct{}
