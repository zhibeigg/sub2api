package qqbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type channelCheckLifecycleRecording struct {
	stages     []string
	errorCodes []string
}

func (r *channelCheckLifecycleRecording) recordChannelCheckLifecycle(stage, _ string, _ Scene, errorCode string) {
	r.stages = append(r.stages, stage)
	r.errorCodes = append(r.errorCodes, errorCode)
}

type failingChannelCheckImageMessenger struct {
	recordingMessenger
}

func (m *failingChannelCheckImageMessenger) SendGroupImage(_ context.Context, _, _, _, _ string, _ uint32) error {
	return errors.New("onebot image action failed")
}

func TestOneBotChannelCheckLifecycleRecordsSafeStages(t *testing.T) {
	settings := defaultBusinessSettings()
	settings.ChannelCheckEnabled = true
	settings.AllowedGroupIDs = []string{"group"}
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{settings: settings})
	channelCheck := &ChannelCheckService{
		settings: channelCheckSettingsStub{runtime: service.ChannelMonitorRuntime{Enabled: true}},
		limiter:  &channelCheckLimiterStub{allowed: true},
		manager:  manager,
		signer:   &ChannelCheckSigner{rootKey: bytes.Repeat([]byte{0x51}, 32)},
	}
	runtime := &Runtime{manager: manager, channelCheck: channelCheck}
	config := ActiveConfig{Enabled: true, AppID: "app", PublicBaseURL: "https://status.example.com"}
	event := InboundEvent{EventID: "event-private-token", MessageID: "message", Scene: SceneGroup, SourceID: "group", ProviderSubject: "user", Content: "/check"}

	t.Run("records recognition URL issuance and successful image action", func(t *testing.T) {
		recorder := &channelCheckLifecycleRecording{}
		if err := runtime.processWith(t.Context(), config, &recordingMessenger{}, nil, event, nil, recorder); err != nil {
			t.Fatal(err)
		}
		if got, want := recorder.stages, []string{channelCheckStageRecognized, channelCheckStageURLIssued, channelCheckStageImageActionSent}; !equalStrings(got, want) {
			t.Fatalf("stages=%v want=%v", got, want)
		}
		if got, want := recorder.errorCodes, []string{"", "", ""}; !equalStrings(got, want) {
			t.Fatalf("error_codes=%v want=%v", got, want)
		}
	})

	t.Run("records URL issuance failure", func(t *testing.T) {
		disabledSettings := defaultBusinessSettings()
		disabledSettings.ChannelCheckEnabled = false
		disabledSettings.AllowedGroupIDs = []string{"group"}
		disabledManager := &ConfigManager{}
		disabledManager.snapshot.Store(&configSnapshot{settings: disabledSettings})
		failedRuntime := &Runtime{manager: disabledManager, channelCheck: &ChannelCheckService{manager: disabledManager, limiter: &channelCheckLimiterStub{allowed: true}, signer: &ChannelCheckSigner{rootKey: bytes.Repeat([]byte{0x52}, 32)}}}
		recorder := &channelCheckLifecycleRecording{}
		if err := failedRuntime.processWith(t.Context(), config, &recordingMessenger{}, nil, event, nil, recorder); err != nil {
			t.Fatal(err)
		}
		if got, want := recorder.stages, []string{channelCheckStageRecognized, channelCheckStagePrepareFailed}; !equalStrings(got, want) {
			t.Fatalf("stages=%v want=%v", got, want)
		}
		if got, want := recorder.errorCodes, []string{"", "channel_check_disabled"}; !equalStrings(got, want) {
			t.Fatalf("error_codes=%v want=%v", got, want)
		}
	})

	t.Run("records image action failure without exposing the image URL", func(t *testing.T) {
		recorder := &channelCheckLifecycleRecording{}
		err := runtime.processWith(t.Context(), config, &failingChannelCheckImageMessenger{}, nil, event, nil, recorder)
		if err == nil {
			t.Fatal("expected image action error")
		}
		if got, want := recorder.stages, []string{channelCheckStageRecognized, channelCheckStageURLIssued, channelCheckStageImageFailed}; !equalStrings(got, want) {
			t.Fatalf("stages=%v want=%v", got, want)
		}
		if got, want := recorder.errorCodes, []string{"", "", "channel_check_action_failed"}; !equalStrings(got, want) {
			t.Fatalf("error_codes=%v want=%v", got, want)
		}
	})
}

func TestOneBotRuntimeChannelCheckDiagnosticsDoNotExposeEventData(t *testing.T) {
	runtime := &OneBotRuntime{state: RuntimeState{ProcessStatus: RuntimeRunning}}
	runtime.recordChannelCheckLifecycle(channelCheckStageRecognized, "event-private-token", SceneGroup, "")
	runtime.recordChannelCheckLifecycle(channelCheckStageURLIssued, "event-private-token", SceneGroup, "")
	runtime.recordChannelCheckLifecycle(channelCheckStageImageFailed, "event-private-token", SceneGroup, "channel_check_action_failed")

	state := runtime.State(t.Context())
	if state.ChannelCheckDiagnostics.LastRecognizedAt == nil || state.ChannelCheckDiagnostics.LastURLIssuedAt == nil || state.ChannelCheckDiagnostics.LastFailureAt == nil {
		t.Fatalf("diagnostics=%#v", state.ChannelCheckDiagnostics)
	}
	if state.ChannelCheckDiagnostics.LastFailureStage != channelCheckStageImageFailed || state.ChannelCheckDiagnostics.LastFailureErrorCode != "channel_check_action_failed" {
		t.Fatalf("diagnostics=%#v", state.ChannelCheckDiagnostics)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"event-private-token", "https://", "group", "user"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("diagnostics exposed %q in %s", forbidden, encoded)
		}
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
