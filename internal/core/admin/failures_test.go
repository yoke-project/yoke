package admin_test

import (
	"testing"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
)

// std: yoke:the-operations.11
func TestAnUndeclaredQuestionAndAUnitsErrorAreRefusedAsWhatTheyAre(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{"acquire": "failing"})
	ask := func(typ string) *administrativev1.Refusal {
		_, ref := b.call(t, v1(&administrativev1.Request{Operation: &administrativev1.Request_UnitAsk{UnitAsk: &administrativev1.UnitAsk{
			Unit: "acquire", Type: typ, Question: []byte("?")}}}))
		return ref
	}
	if ref := ask("undeclared.type"); ref.GetCode() != "scope.undeclared" || ref.GetItem() != "undeclared.type" {
		t.Errorf("a question of an undeclared type was refused %v", ref)
	}
	if ref := ask("head-status"); ref.GetCode() != "unit.failed" || ref.GetItem() != "instrument.busy" || ref.GetMessage() != "the lamp is warming" {
		t.Errorf("a question the unit failed was refused %v", ref)
	}
}
