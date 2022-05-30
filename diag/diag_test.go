// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package diag_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	diag2 "git.tcp.direct/kayos/rout5/diag"
)

func TestDiagLink(t *testing.T) {
	if _, err := diag2.Link("nonexistant").Evaluate(); err == nil {
		t.Errorf("Link(nonexistant).Evaluate = nil, want non-nil")
	}

	if _, err := diag2.Link("lo").Evaluate(); err != nil {
		t.Errorf("Link(lo).Evaluate = %v, want nil", err)
	}
}

func TestDiagMonitor(t *testing.T) {
	m := diag2.NewMonitor(diag2.Link("nonexistant").
		Then(diag2.DHCPv4()))
	got := m.Evaluate()
	want := &diag2.EvalResult{
		Name:   "link/nonexistant",
		Error:  true,
		Status: "Link not found",
		Children: []*diag2.EvalResult{
			{
				Name:   "dhcp4",
				Error:  true,
				Status: "dependency link/nonexistant failed",
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("Evaluate(): unexpected result: diff (-want +got):\n%s", diff)
	}
}
