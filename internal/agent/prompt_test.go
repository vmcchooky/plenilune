package agent

import (
	"strings"
	"testing"
)

func TestRepairInstructionFollowsToolAvailability(t *testing.T) {
	payload, err := buildPayload(testInput())
	if err != nil {
		t.Fatal(err)
	}
	instructions := string(payload)
	if !strings.Contains(instructions, "use verify_patch only while it is in the tool list") {
		t.Fatal("payload does not tie repair to the tool list")
	}
	if strings.Contains(instructions, "repair pins are present") {
		t.Fatal("payload still treats pins as the repair-availability signal")
	}
	if !strings.Contains(systemPrompt, "if it is not in the tool list, do not propose repairs") {
		t.Fatal("system prompt does not describe the tool-list gate")
	}
}

func TestVerifyPatchIsOfferedOnlyWhenRepairIsAvailable(t *testing.T) {
	for _, available := range []bool{false, true} {
		in := testInput()
		in.RepairAvailable = available
		provider := scripted(contentResponse(proposal), contentResponse(finalHolds))
		s := newSession(t, provider, &fakeEngine{})
		res := runSession(t, s, in)
		if res.Status != StatusCompleted {
			t.Fatalf("repair=%t result = %s/%s", available, res.Status, res.Reason)
		}
		offered := false
		for _, tool := range provider.request(0).Tools {
			if tool.Function.Name == "verify_patch" {
				offered = true
			}
		}
		if offered != available {
			t.Fatalf("repair=%t verify_patch offered=%t", available, offered)
		}
	}
}
