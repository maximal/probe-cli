package model

//
// Definition of experiment and types used by the
// implementation of all experiments.
//

import (
	"context"
	"errors"

	"github.com/ooni/probe-cli/v3/internal/humanize"
)

// ExperimentSession is the experiment's view of a session.
type ExperimentSession interface {
	// GetTestHelpersByName returns a list of test helpers with the given name.
	GetTestHelpersByName(name string) ([]OOAPIService, bool)

	// DefaultHTTPClient returns the default HTTPClient used by the session.
	DefaultHTTPClient() HTTPClient

	// FetchPsiphonConfig returns psiphon's config as a serialized JSON or an error.
	FetchPsiphonConfig(ctx context.Context) ([]byte, error)

	// FetchTorTargets returns the targets for the Tor experiment or an error.
	FetchTorTargets(ctx context.Context, cc string) (map[string]OOAPITorTarget, error)

	// Logger returns the logger used by the session.
	Logger() Logger

	// ProbeCC returns the country code.
	ProbeCC() string

	// ResolverIP returns the resolver's IP.
	ResolverIP() string

	// TempDir returns the session's temporary directory.
	TempDir() string

	// TorArgs returns the arguments we should pass to tor when executing it.
	TorArgs() []string

	// TorBinary returns the path of the tor binary.
	TorBinary() string

	// TunnelDir is the directory where to store tunnel information.
	TunnelDir() string

	// UserAgent returns the user agent we should be using when we're fine
	// with identifying ourselves as ooniprobe.
	UserAgent() string
}

// ExperimentAsyncTestKeys is the type of test keys returned by an experiment
// when running in async fashion rather than in sync fashion.
type ExperimentAsyncTestKeys struct {
	// Extensions contains the extensions used by this experiment.
	Extensions map[string]int64

	// Input is the input this measurement refers to.
	Input MeasurementTarget

	// MeasurementRuntime is the total measurement runtime.
	MeasurementRuntime float64

	// TestHelpers contains the test helpers used in the experiment
	TestHelpers map[string]interface{}

	// TestKeys contains the actual test keys.
	TestKeys interface{}
}

// ExperimentMeasurerAsync is a measurer that can run in async fashion.
//
// Currently this functionality is optional, but we will likely
// migrate all experiments to use this functionality in 2022.
type ExperimentMeasurerAsync interface {
	// RunAsync runs the experiment in async fashion.
	//
	// Arguments:
	//
	// - ctx is the context for deadline/timeout/cancellation
	//
	// - sess is the measurement session
	//
	// - input is the input URL to measure
	//
	// - callbacks contains the experiment callbacks
	//
	// Returns either a channel where TestKeys are posted or an error.
	//
	// An error indicates that specific preconditions for running the experiment
	// are not met (e.g., the input URL is invalid).
	//
	// On success, the experiment will post on the channel each new
	// measurement until it is done and closes the channel.
	RunAsync(ctx context.Context, sess ExperimentSession, input string,
		callbacks ExperimentCallbacks) (<-chan *ExperimentAsyncTestKeys, error)
}

// ExperimentCallbacks contains callbacks invoked when experiment events occur.
type ExperimentCallbacks interface {
	// OnProgress provides information about an experiment progress.
	//
	// Arguments:
	//
	// - percentage is a number between 0 and 1 indicating the current progress
	//
	// - message is a string message associated with the progress
	OnProgress(percentage float64, message string)

	// OnData provides information about the data usage.
	//
	// Arguments:
	//
	// - kibiBytesSent is the number of KiB sent by the experiment
	//
	// - kibiBytesReceived is the number of KiB received by the experiment
	OnData(kibiBytesSent, kibiBytesReceived float64)

	// OnMeasurementSubmission provides information about measurement submission.
	//
	// Arguments:
	//
	// - idx is the index of this measurement
	//
	// - m is the measurement
	//
	// - err is the submission error
	//
	// When submission is disabled the err value is ErrSubmissionDisabled.
	OnMeasurementSubmission(idx int, m *Measurement, err error)
}

// ErrSubmissionDisabled indicates that the user has disabled measurements submission.
var ErrSubmissionDisabled = errors.New("submission_disabled_error")

// PrinterCallbacks is the default event handler
type PrinterCallbacks struct {
	Logger
}

var _ ExperimentCallbacks = PrinterCallbacks{}

// NewPrinterCallbacks returns a new default callback handler
func NewPrinterCallbacks(logger Logger) PrinterCallbacks {
	return PrinterCallbacks{Logger: logger}
}

// OnProgress provides information about an experiment progress.
func (d PrinterCallbacks) OnProgress(percentage float64, message string) {
	d.Logger.Infof("[%5.1f%%] %s", percentage*100, message)
}

// OnData implements ExperimentCallbacks.OnData.
func (d PrinterCallbacks) OnData(kibiBytesSent, kibiBytesReceived float64) {
	d.Infof(
		"experiment: recv %s, sent %s",
		humanize.SI(kibiBytesReceived*1024, "byte"),
		humanize.SI(kibiBytesSent*1024, "byte"),
	)
}

// OnMeasurementSubmission implements ExperimentCallbacks
func (PrinterCallbacks) OnMeasurementSubmission(idx int, m *Measurement, err error) {
	// nothing
}

// ExperimentMeasurer is the interface that allows to run a
// measurement for a specific experiment.
type ExperimentMeasurer interface {
	// ExperimentName returns the experiment name.
	ExperimentName() string

	// ExperimentVersion returns the experiment version.
	ExperimentVersion() string

	// Run runs the experiment with the specified context, session,
	// measurement, and experiment calbacks. This method should only
	// return an error in case the experiment could not run (e.g.,
	// a required input is missing). Otherwise, the code should just
	// set the relevant OONI error inside of the measurement and
	// return nil. This is important because the caller WILL NOT submit
	// the measurement if this method returns an error.
	Run(
		ctx context.Context, sess ExperimentSession,
		measurement *Measurement, callbacks ExperimentCallbacks,
	) error

	// GetSummaryKeys returns summary keys expected by ooni/probe-cli.
	GetSummaryKeys(*Measurement) (interface{}, error)
}
