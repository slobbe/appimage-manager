package app

import "context"

type ActivityReporter interface {
	Start(ctx context.Context, activity Activity) ActivityTask
}

type ActivityKind string

type ActivityUnit string

const (
	ActivityKindUnknown         ActivityKind = "unknown"
	ActivityKindCheckingGitHub  ActivityKind = "checking-github"
	ActivityKindIntegrating     ActivityKind = "integrating"
	ActivityKindRemoving        ActivityKind = "removing"
	ActivityKindCheckingUpdates ActivityKind = "checking-updates"
	ActivityKindWaiting         ActivityKind = "waiting"
	ActivityKindDownloading     ActivityKind = "downloading"
)

const (
	ActivityUnitDefault ActivityUnit = ""
	ActivityUnitBytes   ActivityUnit = "bytes"
)

type Activity struct {
	Kind ActivityKind

	AppID     string
	Path      string
	Repo      string
	AssetName string

	// Total is the total number of units for determinate progress.
	// If Total <= 0, the task is indeterminate and can be rendered as a spinner.
	Total int64
	Unit  ActivityUnit
}

type ActivityTask interface {
	Message(message string)
	Advance(delta int64)
	Set(current int64)
	Done(message string)
	Fail(err error)
}

type NoopActivityReporter struct{}

func (NoopActivityReporter) Start(ctx context.Context, activity Activity) ActivityTask {
	return NoopActivityTask{}
}

type NoopActivityTask struct{}

func (NoopActivityTask) Message(message string) {}
func (NoopActivityTask) Advance(delta int64)    {}
func (NoopActivityTask) Set(current int64)      {}
func (NoopActivityTask) Done(message string)    {}
func (NoopActivityTask) Fail(err error)         {}
