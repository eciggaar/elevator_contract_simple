/*
Copyright (c) 2016 IBM Corporation and other Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and limitations under the License.

Contributors:
Rahul Gupta - World of Watson 2016
Leucir Marin - World of Watson 2016
*/ 

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
    "strings"
    "reflect"
	"github.com/hyperledger/fabric/core/chaincode/shim"
)

//go:generate go run scripts/generate_go_schema.go


//***************************************************
//***************************************************
//* CONTRACT initialization and runtime engine
//***************************************************
//***************************************************

// ************************************
// definitions 
// ************************************

// SimpleChaincode is the receiver for all shim API
type SimpleChaincode struct {
}

// ASSETID is the JSON tag for the assetID
const ASSETID string = "assetID"
// TIMESTAMP is the JSON tag for timestamps, devices must use this tag to be compatible! 
const TIMESTAMP string = "timestamp"
// ArgsMap is a generic map[string]interface{} to be used as a receiver 
type ArgsMap map[string]interface{} 

var log = NewContractLogger(DEFAULTNICKNAME, DEFAULTLOGGINGLEVEL)

// ************************************
// start the message pumps 
// ************************************
func main() {
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		log.Infof("ERROR starting Simple Chaincode: %s", err)
	}
}

// Init is called in deploy mode when contract is initialized
func (t *SimpleChaincode) Init(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
    var stateArg ContractState
	var err error

	log.Info("Entering INIT")
    
    if len(args) != 1 {
        err = errors.New("init expects one argument, a JSON string with  mandatory version and optional nickname") 
		log.Critical(err)
		return nil, err 
	}

	err = json.Unmarshal([]byte(args[0]), &stateArg)
	if err != nil {
        err = fmt.Errorf("Version argument unmarshal failed: %s", err)
        log.Critical(err)
		return nil, err 
	}
    
    if stateArg.Nickname == "" {
        stateArg.Nickname = DEFAULTNICKNAME
    } 

	(*log).setModule(stateArg.Nickname)
    
    err = initializeContractState(stub, stateArg.Version, stateArg.Nickname)
    if err != nil {
        return nil, err
    }
    
    log.Info("Contract initialized")
	return nil, nil
}

// Invoke is called in invoke mode to delegate state changing function messages 
func (t *SimpleChaincode) Invoke(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	if function == "createAsset" {
		return t.createAsset(stub, args)
	} else if function == "updateAsset" {
		return t.updateAsset(stub, args)
	} else if function == "deleteAsset" {
		return t.deleteAsset(stub, args)
	} else if function == "deleteAllAssets" {
		return t.deleteAllAssets(stub, args)
	} else if function == "deletePropertiesFromAsset" {
		return t.deletePropertiesFromAsset(stub, args)
	} else if function == "setLoggingLevel" {
		return nil, t.setLoggingLevel(stub, args)
	} else if function == "setCreateOnUpdate" {
		return nil, t.setCreateOnUpdate(stub, args)
	}
	err := fmt.Errorf("Invoke received unknown invocation: %s", function)
    log.Warning(err)
	return nil, err
}

// Query is called in query mode to delegate non-state-changing queries
func (t *SimpleChaincode) Query(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	if function == "readAsset" {
		return t.readAsset(stub, args)
    } else if function == "readAllAssets" {
		return t.readAllAssets(stub, args)
	} else if function == "readRecentStates" {
		return readRecentStates(stub)
	} else if function == "readAssetHistory" {
		return t.readAssetHistory(stub, args)
	} else if function == "readAssetSamples" {
		return t.readAssetSamples(stub, args)
	} else if function == "readAssetSchemas" {
		return t.readAssetSchemas(stub, args)
	} else if function == "readContractObjectModel" {
		return t.readContractObjectModel(stub, args)
	} else if function == "readContractState" {
		return t.readContractState(stub, args)
	}
	err := fmt.Errorf("Query received unknown invocation: %s", function)
    log.Warning(err)
	return nil, err
}


//***************************************************
//***************************************************
//* ASSET CRUD INTERFACE
//***************************************************
//***************************************************

// ************************************
// createAsset 
// ************************************
func (t *SimpleChaincode) createAsset(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	var assetID string
    var argsMap ArgsMap
	var event interface{}
    var found bool
	var err error
    var timeIn time.Time

	log.Info("Entering createAsset")

    // allowing 2 args because updateAsset is allowed to redirect when
    // asset does not exist
	if len(args) < 1 || len(args) > 2 {
        err = errors.New("Expecting one JSON event object")
		log.Error(err)
		return nil, err
	}
    
    assetID = ""
    eventBytes := []byte(args[0])
    log.Debugf("createAsset arg: %s", args[0])

    err = json.Unmarshal(eventBytes, &event)
    if err != nil {
        log.Errorf("createAsset failed to unmarshal arg: %s", err)
		return nil, err
    } 
    
    if event == nil {
        err = errors.New("createAsset unmarshal arg created nil event")
        log.Error(err)
		return nil, err
    }

    argsMap, found = event.(map[string]interface{})
    if !found {
        err := errors.New("createAsset arg is not a map shape")
        log.Error(err)
        return nil, err
    }

    // is assetID present or blank?
    assetIDBytes, found := getObject(argsMap, ASSETID)
    if found {
        assetID, found = assetIDBytes.(string) 
        if !found || assetID == "" {
            err := errors.New("createAsset arg does not include assetID")
            log.Error(err)
            return nil, err
        }
    }
    
    found = assetIsActive(stub, assetID)
    if found {
        err := fmt.Errorf("createAsset arg asset %s already exists", assetID)
        log.Error(err)
        return nil, err
    }

    // test and set timestamp
    // TODO get time from the shim as soon as they support it, we cannot
    // get consensus now because the timestamp is different on all peers.
    //*************************************************//
    // Suma quick fix for timestamp  - Aug 1
    var timeOut = time.Now() // temp initialization of time variable - not really needed.. keeping old line
    timeInBytes, found := getObject(argsMap, TIMESTAMP)
    
    if found {
        timeIn, found = timeInBytes.(time.Time)
        if found && !timeIn.IsZero() {
            timeOut = timeIn
        }
    }
    txnunixtime, err := stub.GetTxTimestamp()
	if err != nil {
		err = fmt.Errorf("Error getting transaction timestamp: %s", err)
        log.Error(err)
        return nil, err
	}
    txntimestamp := time.Unix(txnunixtime.Seconds, int64(txnunixtime.Nanos))
    timeOut = txntimestamp
    //*************************************************//
    argsMap[TIMESTAMP] = timeOut
    
    // run the rules and raise or clear alerts
    alerts := newAlertStatus()
    if argsMap.executeRules(&alerts) {
        // NOT compliant!
        log.Noticef("createAsset assetID %s is noncompliant", assetID)
        argsMap["alerts"] = alerts
        delete(argsMap, "incompliance")
    } else {
        if alerts.AllClear() {
            // all false, no need to appear
            delete(argsMap, "alerts")
        } else {
            argsMap["alerts"] = alerts
        }
        argsMap["incompliance"] = true
    }
    
    // copy incoming event to outgoing state
    // this contract respects the fact that createAsset can accept a partial state
    // as the moral equivalent of one or more discrete events
    // further: this contract understands that its schema has two discrete objects
    // that are meant to be used to send events: common, and custom
    stateOut := argsMap
    
    // save the original event
    stateOut["lastEvent"] = make(map[string]interface{})
    stateOut["lastEvent"].(map[string]interface{})["function"] = "createAsset"
    stateOut["lastEvent"].(map[string]interface{})["args"] = args[0]
    if len(args) == 2 {
        // in-band protocol for redirect
        stateOut["lastEvent"].(map[string]interface{})["redirectedFromFunction"] = args[1]
    }

    // marshal to JSON and write
    stateJSON, err := json.Marshal(&stateOut)
    if err != nil {
        err := fmt.Errorf("createAsset state for assetID %s failed to marshal", assetID)
        log.Error(err)
        return nil, err
    }

    // finally, put the new state
    log.Infof("Putting new asset state %s to ledger", string(stateJSON))
    err = stub.PutState(assetID, []byte(stateJSON))
    if err != nil {
        err = fmt.Errorf("createAsset AssetID %s PUTSTATE failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }
    log.Infof("createAsset AssetID %s state successfully written to ledger: %s", assetID, string(stateJSON))

    // add asset to contract state
    err = addAssetToContractState(stub, assetID)
    if err != nil {
        err := fmt.Errorf("createAsset asset %s failed to write asset state: %s", assetID, err)
        log.Critical(err)
        return nil, err 
    }

    err = pushRecentState(stub, string(stateJSON))
    if err != nil {
        err = fmt.Errorf("createAsset AssetID %s push to recentstates failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }

    // save state history
    err = createStateHistory(stub, assetID, string(stateJSON))
    if err != nil {
        err := fmt.Errorf("createAsset asset %s state history save failed: %s", assetID, err)
        log.Critical(err)
        return nil, err 
    }
    
	return nil, nil
}

// ************************************
// updateAsset 
// ************************************
func (t *SimpleChaincode) updateAsset(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	var assetID string
	var argsMap ArgsMap
	var event interface{}
	var ledgerMap ArgsMap
	var ledgerBytes interface{}
	var found bool
	var err error
    var timeIn time.Time
    
	log.Info("Entering updateAsset")

	if len(args) != 1 {
        err = errors.New("Expecting one JSON event object")
		log.Error(err)
		return nil, err
	}
    
    assetID = ""
    eventBytes := []byte(args[0])
    log.Debugf("updateAsset arg: %s", args[0])
    
    
    err = json.Unmarshal(eventBytes, &event)
    if err != nil {
        log.Errorf("updateAsset failed to unmarshal arg: %s", err)
        return nil, err
    }

    if event == nil {
        err = errors.New("createAsset unmarshal arg created nil event")
        log.Error(err)
		return nil, err
    }

    argsMap, found = event.(map[string]interface{})
    if !found {
        err := errors.New("updateAsset arg is not a map shape")
        log.Error(err)
        return nil, err
    }
    
    // is assetID present or blank?
    assetIDBytes, found := getObject(argsMap, ASSETID)
    if found {
        assetID, found = assetIDBytes.(string) 
        if !found || assetID == "" {
            err := errors.New("updateAsset arg does not include assetID")
            log.Error(err)
            return nil, err
        }
    }
    log.Noticef("updateAsset found assetID %s", assetID)

    found = assetIsActive(stub, assetID)
    if !found {
        // redirect to createAsset with same parameter list
        if canCreateOnUpdate(stub) {
            log.Noticef("updateAsset redirecting asset %s to createAsset", assetID)
            var newArgs = []string{args[0], "updateAsset"}
            return t.createAsset(stub, newArgs)
        }
        err = fmt.Errorf("updateAsset asset %s does not exist", assetID)
        log.Error(err)
        return nil, err
    }

    // test and set timestamp
    // TODO get time from the shim as soon as they support it, we cannot
    // get consensus now because the timestamp is different on all peers.
    
   //*************************************************//
    // Suma quick fix for timestamp  - Aug 1
    var timeOut = time.Now() // temp initialization of time variable - not really needed.. keeping old line
    timeInBytes, found := getObject(argsMap, TIMESTAMP)
    
    if found {
        timeIn, found = timeInBytes.(time.Time)
        if found && !timeIn.IsZero() {
            timeOut = timeIn
        }
    }
    txnunixtime, err := stub.GetTxTimestamp()
	if err != nil {
		err = fmt.Errorf("Error getting transaction timestamp: %s", err)
        log.Error(err)
        return nil, err
	}
    txntimestamp := time.Unix(txnunixtime.Seconds, int64(txnunixtime.Nanos))
    timeOut = txntimestamp
    //*************************************************//
    argsMap[TIMESTAMP] = timeOut
    // **********************************
    // find the asset state in the ledger
    // **********************************
    log.Infof("updateAsset: retrieving asset %s state from ledger", assetID)
    assetBytes, err := stub.GetState(assetID)
    if err != nil {
        log.Errorf("updateAsset assetID %s GETSTATE failed: %s", assetID, err)
        return nil, err
    }

    // unmarshal the existing state from the ledger to theinterface
    err = json.Unmarshal(assetBytes, &ledgerBytes)
    if err != nil {
        log.Errorf("updateAsset assetID %s unmarshal failed: %s", assetID, err)
        return nil, err
    }
    
    // assert the existing state as a map
    ledgerMap, found = ledgerBytes.(map[string]interface{})
    if !found {
        log.Errorf("updateAsset assetID %s LEDGER state is not a map shape", assetID)
        return nil, err
    }
    
    // now add incoming map values to existing state to merge them
    // this contract respects the fact that updateAsset can accept a partial state
    // as the moral equivalent of one or more discrete events
    // further: this contract understands that its schema has two discrete objects
    // that are meant to be used to send events: common, and custom
    // ledger has to have common section
    stateOut := deepMerge(map[string]interface{}(argsMap), 
                          map[string]interface{}(ledgerMap))
    log.Debugf("updateAsset assetID %s merged state: %s", assetID, stateOut)

    // handle compliance section
    alerts := newAlertStatus()
    a, found := stateOut["alerts"] // is there an existing alert state?
    if found {
        // convert to an AlertStatus, which does not work by type assertion
        log.Debugf("updateAsset Found existing alerts state: %s", a)
        // complex types are all untyped interfaces, so require conversion to
        // the structure that is used, but not in the other direction as the
        // type is properly specified
        alerts.alertStatusFromMap(a.(map[string]interface{}))
    }
    // important: rules need access to the entire calculated state 
    if ledgerMap.executeRules(&alerts) {
        // true means noncompliant
        log.Noticef("updateAsset assetID %s is noncompliant", assetID)
        // update ledger with new state, if all clear then delete
        stateOut["alerts"] = alerts
        delete(stateOut, "incompliance")
    } else {
        if alerts.AllClear() {
            // all false, no need to appear
            delete(stateOut, "alerts")
        } else {
            stateOut["alerts"] = alerts
        }
        stateOut["incompliance"] = true
    }
    
    // save the original event
    stateOut["lastEvent"] = make(map[string]interface{})
    stateOut["lastEvent"].(map[string]interface{})["function"] = "updateAsset"
    stateOut["lastEvent"].(map[string]interface{})["args"] = args[0]

    // Write the new state to the ledger
    stateJSON, err := json.Marshal(ledgerMap)
    if err != nil {
        err = fmt.Errorf("updateAsset AssetID %s marshal failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }

    // finally, put the new state
    err = stub.PutState(assetID, []byte(stateJSON))
    if err != nil {
        err = fmt.Errorf("updateAsset AssetID %s PUTSTATE failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }
    err = pushRecentState(stub, string(stateJSON))
    if err != nil {
        err = fmt.Errorf("updateAsset AssetID %s push to recentstates failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }

    // add history state
    err = updateStateHistory(stub, assetID, string(stateJSON))
    if err != nil {
        err = fmt.Errorf("updateAsset AssetID %s push to history failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }

    // NOTE: Contract state is not updated by updateAsset
    
	return nil, nil
}

// ************************************
// deleteAsset 
// ************************************
func (t *SimpleChaincode) deleteAsset(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	var assetID string
	var argsMap ArgsMap
	var event interface{}
    var found bool
	var err error

	if len(args) != 1 {
        err = errors.New("Expecting one JSON state object with an assetID")
		log.Error(err)
		return nil, err
	}
    
    assetID = ""
    eventBytes := []byte(args[0])
    log.Debugf("deleteAsset arg: %s", args[0])

    err = json.Unmarshal(eventBytes, &event)
    if err != nil {
        log.Errorf("deleteAsset failed to unmarshal arg: %s", err)
        return nil, err
    }

    argsMap, found = event.(map[string]interface{})
    if !found {
        err := errors.New("deleteAsset arg is not a map shape")
        log.Error(err)
        return nil, err
    }
    
    // is assetID present or blank?
    assetIDBytes, found := getObject(argsMap, ASSETID)
    if found {
        assetID, found = assetIDBytes.(string) 
        if !found || assetID == "" {
            err := errors.New("deleteAsset arg does not include assetID")
            log.Error(err)
            return nil, err
        }
    }

    found = assetIsActive(stub, assetID)
    if !found {
        err = fmt.Errorf("deleteAsset assetID %s does not exist", assetID)
        log.Error(err)
        return nil, err
    }

    // Delete the key / asset from the ledger
    err = stub.DelState(assetID)
    if err != nil {
        log.Errorf("deleteAsset assetID %s failed DELSTATE", assetID)
        return nil, err
    }
    // remove asset from contract state
    err = removeAssetFromContractState(stub, assetID)
    if err != nil {
        err := fmt.Errorf("deleteAsset asset %s failed to remove asset from contract state: %s", assetID, err)
        log.Critical(err)
        return nil, err 
    }
    // save state history
    err = deleteStateHistory(stub, assetID)
    if err != nil {
        err := fmt.Errorf("deleteAsset asset %s state history delete failed: %s", assetID, err)
        log.Critical(err)
        return nil, err 
    }
    // push the recent state
    err = removeAssetFromRecentState(stub, assetID)
    if err != nil {
        err := fmt.Errorf("deleteAsset asset %s recent state removal failed: %s", assetID, err)
        log.Critical(err)
        return nil, err 
    }
    
	return nil, nil
}

// ************************************
// deletePropertiesFromAsset 
// ************************************
func (t *SimpleChaincode) deletePropertiesFromAsset(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	var assetID string
	var argsMap ArgsMap
	var event interface{}
    var ledgerMap ArgsMap
	var ledgerBytes interface{}
	var found bool
	var err error
    var alerts AlertStatus

	if len(args) < 1 {
        err = errors.New("Not enough arguments. Expecting one JSON object with mandatory AssetID and property name array")
		log.Error(err)
		return nil, err
	}
    eventBytes := []byte(args[0])

    err = json.Unmarshal(eventBytes, &event)
    if err != nil {
        log.Error("deletePropertiesFromAsset failed to unmarshal arg")
        return nil, err
    }
    
    argsMap, found = event.(map[string]interface{})
    if !found {
        err := errors.New("updateAsset arg is not a map shape")
        log.Error(err)
        return nil, err
    }
    log.Debugf("deletePropertiesFromAsset arg: %+v", argsMap)
    
    // is assetID present or blank?
    assetIDBytes, found := getObject(argsMap, ASSETID)
    if found {
        assetID, found = assetIDBytes.(string) 
        if !found || assetID == "" {
            err := errors.New("deletePropertiesFromAsset arg does not include assetID")
            log.Error(err)
            return nil, err
        }
    }

    found = assetIsActive(stub, assetID)
    if !found {
        err = fmt.Errorf("deletePropertiesFromAsset assetID %s does not exist", assetID)
        log.Error(err)
        return nil, err
    }

    // is there a list of property names?
    var qprops []interface{}
    qpropsBytes, found := getObject(argsMap, "qualPropsToDelete")
    if found {
        qprops, found = qpropsBytes.([]interface{})
        log.Debugf("deletePropertiesFromAsset qProps: %+v, Found: %+v, Type: %+v", qprops, found, reflect.TypeOf(qprops))
        if !found || len(qprops) < 1 {
            log.Errorf("deletePropertiesFromAsset asset %s qualPropsToDelete is not an array or is empty", assetID)
            return nil, err
        }
    } else {
        log.Errorf("deletePropertiesFromAsset asset %s has no qualPropsToDelete argument", assetID)
        return nil, err
    }

    // **********************************
    // find the asset state in the ledger
    // **********************************
    log.Infof("deletePropertiesFromAsset: retrieving asset %s state from ledger", assetID)
    assetBytes, err := stub.GetState(assetID)
    if err != nil {
        err = fmt.Errorf("deletePropertiesFromAsset AssetID %s GETSTATE failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }

    // unmarshal the existing state from the ledger to the interface
    err = json.Unmarshal(assetBytes, &ledgerBytes)
    if err != nil {
        err = fmt.Errorf("deletePropertiesFromAsset AssetID %s unmarshal failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }
    
    // assert the existing state as a map
    ledgerMap, found = ledgerBytes.(map[string]interface{})
    if !found {
        err = fmt.Errorf("deletePropertiesFromAsset AssetID %s LEDGER state is not a map shape", assetID)
        log.Error(err)
        return nil, err
    }

    // now remove properties from state, they are qualified by level
    OUTERDELETELOOP:
    for p := range qprops {
        prop := qprops[p].(string)
        log.Debugf("deletePropertiesFromAsset AssetID %s deleting qualified property: %s", assetID, prop)
        // TODO Ugly, isolate in a function at some point
        if (CASESENSITIVEMODE  && strings.HasSuffix(prop, ASSETID)) ||
           (!CASESENSITIVEMODE && strings.HasSuffix(strings.ToLower(prop), strings.ToLower(ASSETID))) {
            log.Warningf("deletePropertiesFromAsset AssetID %s cannot delete protected qualified property: %s", assetID, prop)
        } else {
            levels := strings.Split(prop, ".")
            lm := (map[string]interface{})(ledgerMap)
            for l := range levels {
                // lev is the name of a level
                lev := levels[l]
                if l == len(levels)-1 {
                    // we're here, delete the actual property name from this level of the map
                    levActual, found := findMatchingKey(lm, lev)
                    if !found {
                        log.Warningf("deletePropertiesFromAsset AssetID %s property match %s not found", assetID, lev)
                        continue OUTERDELETELOOP
                    }
                    log.Debugf("deletePropertiesFromAsset AssetID %s deleting %s", assetID, prop)
                    delete(lm, levActual)
                } else {
                    // navigate to the next level object
                    log.Debugf("deletePropertiesFromAsset AssetID %s navigating to level %s", assetID, lev)
                    lmBytes, found := findObjectByKey(lm, lev)
                    if found {
                        lm, found = lmBytes.(map[string]interface{})
                        if !found {
                            log.Noticef("deletePropertiesFromAsset AssetID %s level %s not found in ledger", assetID, lev)
                            continue OUTERDELETELOOP
                        }
                    } 
                }
            } 
        }
    }
    log.Debugf("updateAsset AssetID %s final state: %s", assetID, ledgerMap)

    // set timestamp
    // TODO timestamp from the stub
    //ledgerMap[TIMESTAMP] = time.Now()
    //*************************************************//
    // Suma quick fix for timestamp  - Aug 1
     txnunixtime, err := stub.GetTxTimestamp()
	if err != nil {
		err = fmt.Errorf("Error getting transaction timestamp: %s", err)
        log.Error(err)
        return nil, err
	}
    txntimestamp := time.Unix(txnunixtime.Seconds, int64(txnunixtime.Nanos))
    ledgerMap[TIMESTAMP] = txntimestamp
    //*************************************************//
    // handle compliance section
    alerts = newAlertStatus()
    a, found := argsMap["alerts"] // is there an existing alert state?
    if found {
        // convert to an AlertStatus, which does not work by type assertion
        log.Debugf("deletePropertiesFromAsset Found existing alerts state: %s", a)
        // complex types are all untyped interfaces, so require conversion to
        // the structure that is used, but not in the other direction as the
        // type is properly specified
        alerts.alertStatusFromMap(a.(map[string]interface{}))
    }
    // important: rules need access to the entire calculated state 
    if ledgerMap.executeRules(&alerts) {
        // true means noncompliant
        log.Noticef("deletePropertiesFromAsset assetID %s is noncompliant", assetID)
        // update ledger with new state, if all clear then delete
        ledgerMap["alerts"] = alerts
        delete(ledgerMap, "incompliance")
    } else {
        if alerts.AllClear() {
            // all false, no need to appear
            delete(ledgerMap, "alerts")
        } else {
            ledgerMap["alerts"] = alerts
        }
        ledgerMap["incompliance"] = true
    }
    
    // save the original event
    ledgerMap["lastEvent"] = make(map[string]interface{})
    ledgerMap["lastEvent"].(map[string]interface{})["function"] = "deletePropertiesFromAsset"
    ledgerMap["lastEvent"].(map[string]interface{})["args"] = args[0]
    
    // Write the new state to the ledger
    stateJSON, err := json.Marshal(ledgerMap)
    if err != nil {
        err = fmt.Errorf("deletePropertiesFromAsset AssetID %s marshal failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }

    // finally, put the new state
    err = stub.PutState(assetID, []byte(stateJSON))
    if err != nil {
        err = fmt.Errorf("deletePropertiesFromAsset AssetID %s PUTSTATE failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }
    err = pushRecentState(stub, string(stateJSON))
    if err != nil {
        err = fmt.Errorf("deletePropertiesFromAsset AssetID %s push to recentstates failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }

    // add history state
    err = updateStateHistory(stub, assetID, string(stateJSON))
    if err != nil {
        err = fmt.Errorf("deletePropertiesFromAsset AssetID %s push to history failed: %s", assetID, err)
        log.Error(err)
        return nil, err
    }

	return nil, nil
}

// ************************************
// deletaAllAssets 
// ************************************
func (t *SimpleChaincode) deleteAllAssets(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	var assetID string
	var err error

	if len(args) > 0 {
        err = errors.New("Too many arguments. Expecting none.")
		log.Error(err)
		return nil, err
	}
    
    aa, err := getActiveAssets(stub)
    if err != nil {
        err = fmt.Errorf("deleteAllAssets failed to get the active assets: %s", err)
        log.Error(err)
        return nil, err
    }
    for i := range aa {
        assetID = aa[i]
        
        // Delete the key / asset from the ledger
        err = stub.DelState(assetID)
        if err != nil {
            err = fmt.Errorf("deleteAllAssets arg %d assetID %s failed DELSTATE", i, assetID)
            log.Error(err)
            return nil, err
        }
        // remove asset from contract state
        err = removeAssetFromContractState(stub, assetID)
        if err != nil {
            err = fmt.Errorf("deleteAllAssets asset %s failed to remove asset from contract state: %s", assetID, err)
            log.Critical(err)
            return nil, err 
        }
        // save state history
        err = deleteStateHistory(stub, assetID)
        if err != nil {
            err := fmt.Errorf("deleteAllAssets asset %s state history delete failed: %s", assetID, err)
            log.Critical(err)
            return nil, err 
        }
    }
    err = clearRecentStates(stub)
    if err != nil {
        err = fmt.Errorf("deleteAllAssets clearRecentStates failed: %s", err)
        log.Error(err)
        return nil, err
    }
	return nil, nil
}

// ************************************
// readAsset 
// ************************************
func (t *SimpleChaincode) readAsset(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
    var assetID string
	var argsMap ArgsMap
	var request interface{}
    var assetBytes []byte
    var found bool
	var err error
    
	if len(args) != 1 {
        err = errors.New("Expecting one JSON event object")
		log.Error(err)
		return nil, err
	}
    
    requestBytes := []byte(args[0])
    log.Debugf("readAsset arg: %s", args[0])
    
    err = json.Unmarshal(requestBytes, &request)
    if err != nil {
        log.Errorf("readAsset failed to unmarshal arg: %s", err)
		return nil, err
    }

    argsMap, found = request.(map[string]interface{})
    if !found {
        err := errors.New("readAsset arg is not a map shape")
        log.Error(err)
        return nil, err
    }
    
    // is assetID present or blank?
    assetIDBytes, found := getObject(argsMap, ASSETID)
    if found {
        assetID, found = assetIDBytes.(string) 
        if !found || assetID == "" {
            err := errors.New("readAsset arg does not include assetID")
            log.Error(err)
            return nil, err
        }
    }
    
    found = assetIsActive(stub, assetID)
    if !found {
        err := fmt.Errorf("readAsset arg asset %s does not exist", assetID)
        log.Error(err)
        return nil, err
    }

    // Get the state from the ledger
    assetBytes, err = stub.GetState(assetID)
    if err != nil {
        log.Errorf("readAsset assetID %s failed GETSTATE", assetID)
        return nil, err
    } 

	return assetBytes, nil
}

// ************************************
// readAllAssets 
// ************************************
func (t *SimpleChaincode) readAllAssets(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	var assetID string
	var err error
    var results []interface{}
    var state interface{}

	if len(args) > 0 {
        err = errors.New("readAllAssets expects no arguments")
		log.Error(err)
		return nil, err
	}
    
    aa, err := getActiveAssets(stub)
    if err != nil {
        err = fmt.Errorf("readAllAssets failed to get the active assets: %s", err)
		log.Error(err)
        return nil, err
    }
    results = make([]interface{}, 0, len(aa))
    for i := range aa {
        assetID = aa[i]
        // Get the state from the ledger
        assetBytes, err := stub.GetState(assetID)
        if err != nil {
            // best efforts, return what we can
            log.Errorf("readAllAssets assetID %s failed GETSTATE", assetID)
            continue
        } else {
            err = json.Unmarshal(assetBytes, &state)
            if err != nil {
                // best efforts, return what we can
                log.Errorf("readAllAssets assetID %s failed to unmarshal", assetID)
                continue
            }
            results = append(results, state)
        }
    }
    
    resultsStr, err := json.Marshal(results)
    if err != nil {
        err = fmt.Errorf("readallAssets failed to marshal results: %s", err)
        log.Error(err)
        return nil, err
    }

	return []byte(resultsStr), nil
}

// ************************************
// readAssetHistory 
// ************************************
func (t *SimpleChaincode) readAssetHistory(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
    var assetBytes []byte
    var assetID string
	var argsMap ArgsMap
	var request interface{}
    var found bool
	var err error

	if len(args) != 1 {
        err = errors.New("readAssetHistory expects a JSON encoded object with assetID and count")
		log.Error(err)
		return nil, err
	}
    
    requestBytes := []byte(args[0])
    log.Debugf("readAssetHistory arg: %s", args[0])
    
    err = json.Unmarshal(requestBytes, &request)
    if err != nil {
        err = fmt.Errorf("readAssetHistory failed to unmarshal arg: %s", err)
        log.Error(err)
        return nil, err
    }
    
    argsMap, found = request.(map[string]interface{})
    if !found {
        err := errors.New("readAssetHistory arg is not a map shape")
        log.Error(err)
        return nil, err
    }
    
    // is assetID present or blank?
    assetIDBytes, found := getObject(argsMap, ASSETID)
    if found {
        assetID, found = assetIDBytes.(string) 
        if !found || assetID == "" {
            err := errors.New("readAssetHistory arg does not include assetID")
            log.Error(err)
            return nil, err
        }
    }
    
    found = assetIsActive(stub, assetID)
    if !found {
        err := fmt.Errorf("readAssetHistory arg asset %s does not exist", assetID)
        log.Error(err)
        return nil, err
    }

    // Get the history from the ledger
    stateHistory, err := readStateHistory(stub, assetID)
    if err != nil {
        err = fmt.Errorf("readAssetHistory assetID %s failed readStateHistory: %s", assetID, err)
        log.Error(err)
        return nil, err
    }
    
    // is count present?
    var olen int
    countBytes, found := getObject(argsMap, "count")
    if found {
        olen = int(countBytes.(float64))
    }
    if olen <= 0 || olen > len(stateHistory.AssetHistory) { 
        olen = len(stateHistory.AssetHistory) 
    }
    var hStatesOut = make([]interface{}, 0, olen) 
    for i := 0; i < olen; i++ {
        var obj interface{}
        err = json.Unmarshal([]byte(stateHistory.AssetHistory[i]), &obj)
        if err != nil {
            log.Errorf("readAssetHistory JSON unmarshal of entry %d failed [%#v]", i, stateHistory.AssetHistory[i])
            return nil, err
        }
        hStatesOut = append(hStatesOut, obj)
    }
	assetBytes, err = json.Marshal(hStatesOut)
    if err != nil {
        log.Errorf("readAssetHistory failed to marshal results: %s", err)
        return nil, err
    }
    
	return []byte(assetBytes), nil
}


//***************************************************
//***************************************************
//* CONTRACT STATE 
//***************************************************
//***************************************************

func (t *SimpleChaincode) readContractState(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	var err error

	if len(args) != 0 {
        err = errors.New("Too many arguments. Expecting none.")
		log.Error(err)
		return nil, err
	}

	// Get the state from the ledger
	chaincodeBytes, err := stub.GetState(CONTRACTSTATEKEY)
	if err != nil {
        err = fmt.Errorf("readContractState failed GETSTATE: %s", err)
        log.Error(err)
        return nil, err
	}

	return chaincodeBytes, nil
}

//***************************************************
//***************************************************
//* CONTRACT METADATA / SCHEMA INTERFACE
//***************************************************
//***************************************************

// ************************************
// readAssetSamples 
// ************************************
func (t *SimpleChaincode) readAssetSamples(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	return []byte(samples), nil
}

// ************************************
// readAssetSchemas 
// ************************************
func (t *SimpleChaincode) readAssetSchemas(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	return []byte(schemas), nil
}

// ************************************
// readContractObjectModel 
// ************************************
func (t *SimpleChaincode) readContractObjectModel(stub *shim.ChaincodeStub, args []string) ([]byte, error) {
	var state = ContractState{MYVERSION, DEFAULTNICKNAME, make(map[string]bool)}

	stateJSON, err := json.Marshal(state)
	if err != nil {
        err := fmt.Errorf("JSON Marshal failed for get contract object model empty state: %+v with error [%s]", state, err)
        log.Error(err)
		return nil, err
	}
	return stateJSON, nil
}

// ************************************
// setLoggingLevel 
// ************************************
func (t *SimpleChaincode) setLoggingLevel(stub *shim.ChaincodeStub, args []string) (error) {
    type LogLevelArg struct {
        Level string `json:"logLevel"`
    }
    var level LogLevelArg
    var err error
    if len(args) != 1 {
        err = errors.New("Incorrect number of arguments. Expecting a JSON encoded LogLevel.")
		log.Error(err)
		return err
    }
    err = json.Unmarshal([]byte(args[0]), &level)
	if err != nil {
        err = fmt.Errorf("setLoggingLevel failed to unmarshal arg: %s", err)
		log.Error(err)
		return err
    }
    for i, lev := range logLevelNames {
        if strings.ToUpper(level.Level) == lev {
            (*log).SetLoggingLevel(LogLevel(i))
            return nil
        } 
    }
    err = fmt.Errorf("Unknown Logging level: %s", level.Level)
    log.Error(err)
    return err
}

// CreateOnUpdate is a shared parameter structure for the use of 
// the createonupdate feature
type CreateOnUpdate struct {
    CreateOnUpdate bool `json:"createOnUpdate"`
}

// ************************************
// setCreateOnUpdate 
// ************************************
func (t *SimpleChaincode) setCreateOnUpdate(stub *shim.ChaincodeStub, args []string) (error) {
    var createOnUpdate CreateOnUpdate
    var err error
    if len(args) != 1 {
        err = errors.New("setCreateOnUpdate expects a single parameter")
		log.Error(err)
		return err
    }
    err = json.Unmarshal([]byte(args[0]), &createOnUpdate)
	if err != nil {
        err = fmt.Errorf("setCreateOnUpdate failed to unmarshal arg: %s", err)
		log.Error(err)
		return err
    }
    err = PUTcreateOnUpdate(stub, createOnUpdate)
	if err != nil {
        err = fmt.Errorf("setCreateOnUpdate failed to PUT setting: %s", err)
		log.Error(err)
		return err
    }
    return nil
}

// PUTcreateOnUpdate marshals the new setting and writes it to the ledger
func PUTcreateOnUpdate(stub *shim.ChaincodeStub, createOnUpdate CreateOnUpdate) (err error) {
    createOnUpdateBytes, err := json.Marshal(createOnUpdate)
	if err != nil {
        err = errors.New("PUTcreateOnUpdate failed to marshal")
		log.Error(err)
		return err
    }
    err = stub.PutState("CreateOnUpdate", createOnUpdateBytes)
	if err != nil {
        err = fmt.Errorf("PUTSTATE createOnUpdate failed: %s", err)
		log.Error(err)
		return err
    }
    return nil
}

// canCreateOnUpdate retrieves the setting from the ledger and returns it to the calling function
func canCreateOnUpdate(stub *shim.ChaincodeStub) (bool) {
    var createOnUpdate CreateOnUpdate
    createOnUpdateBytes, err := stub.GetState("CreateOnUpdate")
	if err != nil {
        err = fmt.Errorf("GETSTATE for canCreateOnUpdate failed: %s", err)
		log.Error(err)
		return true  // true is the default
    }
    err = json.Unmarshal(createOnUpdateBytes, &createOnUpdate)
	if err != nil {
        err = fmt.Errorf("canCreateOnUpdate failed to marshal: %s", err)
		log.Error(err)
		return true  // true is the default
    }
    return createOnUpdate.CreateOnUpdate
}
