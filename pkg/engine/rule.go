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

package engine

import (
	log "github.com/sirupsen/logrus"
	"reflect"
	"strconv"
	"strings"
)

const (
	ACTION_TYPE_METADATA      = 1
	ACTION_TYPE_DISRUPTIVE    = 2
	ACTION_TYPE_DATA          = 3
	ACTION_TYPE_NONDISRUPTIVE = 4
	ACTION_TYPE_FLOW          = 5

	ACTION_DISRUPTIVE_PASS     = 0
	ACTION_DISRUPTIVE_DROP     = 1
	ACTION_DISRUPTIVE_BLOCK    = 2
	ACTION_DISRUPTIVE_DENY     = 3
	ACTION_DISRUPTIVE_ALLOW    = 4
	ACTION_DISRUPTIVE_PROXY    = 5
	ACTION_DISRUPTIVE_REDIRECT = 6
)

type Action interface {
	Init(*Rule, string) string
	Evaluate(*Rule, *Transaction)
	GetType() int
}

type Operator interface {
	Init(string)
	Evaluate(*Transaction, string) bool
}

type RuleOp struct {
	Operator Operator
	Data     string
	Negation bool
	//OpEval OperatorFunction
}

type RuleTransformation struct {
	Function string
	TfFunc   interface{} `json:"-"`
}

type RuleVariable struct {
	Count      bool
	Collection string
	Key        string
	Exceptions []string
}

type Rule struct {
	// Contains de non-compiled variables part of the rule
	Vars string `json:"vars"`

	Variables               []RuleVariable
	Operator                string
	OperatorObj             *RuleOp
	Disruptive              bool
	Transformations         []RuleTransformation
	HasChain                bool
	ParentId                int
	Actions                 []Action
	ActionParams            string
	MultiMatch              bool
	Severity                string
	Skip                    bool
	SecMark                 string
	Log                     bool
	Raw                     string
	Chain                   *Rule
	DisruptiveAction        int
	DefaultDisruptiveAction string

	//METADATA
	// Rule unique sorted identifier
	Id int

	// Rule tag list
	Tags []string

	// Rule execution phase 1-5
	Phase int

	// Message text to be macro expanded and logged
	Msg string

	// Rule revision value
	Rev string

	// Rule maturity index
	Maturity string

	// Rule Set Version
	Version string

	// Used by deny to create disruption
	Status int
}

func (r *Rule) Init() {
	//It seems default phase for modsec is 2 according to secdefaultaction documentation
	r.Phase = 2
	r.Tags = []string{}
}

func (r *Rule) Evaluate(tx *Transaction) []*MatchData {
	matchedValues := []*MatchData{}
	for _, nid := range tx.RuleRemoveById {
		if nid == r.Id {
			//This rules will be skipped
			return matchedValues
		}
	}
	ecol := tx.GetRemovedTargets(r.Id)
	for _, v := range r.Variables {
		var values []*MatchData
		exceptions := make([]string, len(v.Exceptions))
		copy(exceptions, v.Exceptions)
		if ecol != nil {
			for _, c := range ecol {
				if c.Name == v.Collection {
					exceptions = append(exceptions, c.Key)
				}
			}
		}

		//Get with macro expansion
		values = tx.GetField(v.Collection, v.Key, exceptions)
		if v.Count {
			if v.Key != "" && len(values) == 1 {
				values[0].Value = strconv.Itoa(len(values[0].Value))
			} else {
				values = []*MatchData{
					&MatchData{
						Collection: v.Collection,
						Key:        v.Key,
						Value:      strconv.Itoa(len(values)),
					},
				}
			}
		}

		if len(values) == 0 {
			if r.executeOperator("", tx) {
				matchedValues = append(matchedValues, &MatchData{})
			}
			continue
		}
		log.Debug("Arguments expanded: " + strconv.Itoa(len(values)))
		for _, arg := range values {
			var args []string
			if r.MultiMatch {
				args = r.executeTransformationsMultimatch(arg.Value)
			} else {
				args = []string{r.executeTransformations(arg.Value)}
			}
			log.Debug("Transformed arguments: " + strings.Join(args, ", "))
			for _, carg := range args {
				if r.executeOperator(carg, tx) {
					matchedValues = append(matchedValues, &MatchData{
						Collection: arg.Collection,
						Key:        arg.Key,
						Value:      carg,
					})
				}
			}
		}
	}

	if len(matchedValues) == 0 {
		//No match for variables
		return matchedValues
	}
	// we must match the vars before runing the chains
	tx.MatchVars(matchedValues)

	// We run non disruptive actions even if there is no chain match
	for _, a := range r.Actions {
		if a.GetType() == ACTION_TYPE_NONDISRUPTIVE {
			a.Evaluate(r, tx)
		}
	}

	// We reset the capturable configuration
	tx.SetCapturable(false)

	msgs := []string{tx.MacroExpansion(r.Msg)}
	if r.Chain != nil {
		nr := r.Chain
		for nr != nil {
			m := nr.Evaluate(tx)
			if len(m) == 0 {
				//we fail the chain
				return []*MatchData{}
			}
			msgs = append(msgs, tx.MacroExpansion(nr.Msg))

			for _, child := range m {
				matchedValues = append(matchedValues, child)
			}
			nr = nr.Chain
		}
	}
	if r.ParentId == 0 {
		// action log is required to add the rule to matched rules
		if r.Log {
			tx.MatchRule(r, msgs, matchedValues)
		}
		//we need to add disruptive actions in the end, otherwise they would be triggered without their chains.
		for _, a := range r.Actions {
			if a.GetType() == ACTION_TYPE_DISRUPTIVE || a.GetType() == ACTION_TYPE_FLOW {
				a.Evaluate(r, tx)
			}
		}
	}
	return matchedValues
}

func (r *Rule) executeOperator(data string, tx *Transaction) bool {
	result := r.OperatorObj.Operator.Evaluate(tx, data)
	if r.OperatorObj.Negation && result {
		return false
	}
	if r.OperatorObj.Negation && !result {
		return true
	}
	return result
}

func (r *Rule) executeTransformationsMultimatch(value string) []string {
	res := []string{value}
	for _, t := range r.Transformations {
		rf := reflect.ValueOf(t.TfFunc)
		rargs := make([]reflect.Value, 1)
		rargs[0] = reflect.ValueOf(value)
		call := rf.Call(rargs)
		value = call[0].String()
		res = append(res, value)
	}
	return res
}

func (r *Rule) executeTransformations(value string) string {
	for _, t := range r.Transformations {
		rf := reflect.ValueOf(t.TfFunc)
		rargs := make([]reflect.Value, 1)
		rargs[0] = reflect.ValueOf(value)
		call := rf.Call(rargs)
		value = call[0].String()
	}
	return value
}

func (r *Rule) AddVariable(count bool, collection string, key string) {
	rv := RuleVariable{count, collection, key, []string{}}
	r.Variables = append(r.Variables, rv)
}

func (r *Rule) AddNegateVariable(collection string, key string) {
	for i, vr := range r.Variables {
		if vr.Collection == collection {
			vr.Exceptions = append(vr.Exceptions, key)
			r.Variables[i] = vr
			return
		}
	}
}

func NewRule() *Rule {
	r := &Rule{}
	r.Init()
	return r
}
