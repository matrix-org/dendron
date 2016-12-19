package proxy

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestLogAndReplyError(t *testing.T) {
	w := httptest.NewRecorder()
	LogAndReplyError(w, &HTTPError{
		Err:        errors.New("ignored"),
		StatusCode: 420,
		Message:    "test message",
		ErrCode:    "test errcode",
	})

	if w.Code != 420 {
		t.Errorf("expected statuscode to be 420, was %d", w.Code)
	}
	if w.Body == nil {
		t.Fatalf("no body")
	}
	jsonResp := map[string]string{}
	err := json.Unmarshal(w.Body.Bytes(), &jsonResp)
	if err != nil {
		t.Fatalf("response %q was not valid json: %v", w.Body.String(), err)
	}

	expectedJson := map[string]string{"errcode": "test errcode", "error": "test message"}
	if !reflect.DeepEqual(jsonResp, expectedJson) {
		t.Errorf("expected %v, got %v", expectedJson, jsonResp)
	}
}
