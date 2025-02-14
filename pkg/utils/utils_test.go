// Copyright 2021 Juan Pablo Tosso
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

package utils

import (
	"testing"
)

func TestOpenFile(t *testing.T) {
	b, err := OpenFile("https://github.com/")
	if len(b) == 0 || err != nil {
		t.Error("Failed to read remote file with OpenFile")
	}

	b, err = OpenFile("../../readme.md")
	if len(b) == 0 || err != nil {
		t.Error("Failed to read local file with OpenFile")
	}
}

func TestRandomString(t *testing.T) {
	s1 := RandomString(10)
	s2 := RandomString(10)
	if len(s1)+len(s2) != 20 {
		t.Error("Failed to generate random string")
	}

	if s1 == s2 {
		t.Error("Failed to generate entropy")
	}
}
