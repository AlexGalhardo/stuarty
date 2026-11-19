package shell

import (
	"encoding/json"
	"strconv"
)

// The narrow bridge of SDD-012 section 13.1 item 2: window.vukemDesktop, the
// only coupling between the editor and the shell. It carries file paths, never file
// bytes and never an editing rule. The page sends a
// JSON message through the webview, the shell answers by running ResolveScript.

// The operations the page may ask for; nothing else reaches the shell.
const (
	OpPickFiles    = "pickFiles"
	OpPickTemplate = "pickTemplate"
	OpShowInFolder = "showInFolder"
)

// maxBridgeMessage bounds a message; a path is the longest thing it carries.
const maxBridgeMessage = 4096

// BridgeRequest is one message from the page. Path is set only for OpShowInFolder.
type BridgeRequest struct {
	Op   string `json:"vukem"`
	ID   int64  `json:"id"`
	Path string `json:"path,omitempty"`
}

// ParseBridgeMessage decodes a raw webview message. It reports false for
// anything that is not a known request, so a page cannot reach any other code.
func ParseBridgeMessage(msg string) (BridgeRequest, bool) {
	var req BridgeRequest
	if len(msg) > maxBridgeMessage || json.Unmarshal([]byte(msg), &req) != nil {
		return BridgeRequest{}, false
	}
	switch req.Op {
	case OpPickFiles, OpPickTemplate:
		req.Path = ""
	case OpShowInFolder:
		if req.Path == "" {
			return BridgeRequest{}, false
		}
	default:
		return BridgeRequest{}, false
	}
	return req, true
}

// ResolveScript settles the promise of request id: with paths, or with an
// error message when errMsg is not empty.
func ResolveScript(id int64, paths []string, errMsg string) string {
	if paths == nil {
		paths = []string{}
	}
	p, err := json.Marshal(paths)
	if err != nil {
		p = []byte("[]")
	}
	e, err := json.Marshal(errMsg)
	if err != nil {
		e = []byte(`"error"`)
	}
	return `window._wails&&window._wails.vukemResolve&&window._wails.vukemResolve(` +
		strconv.FormatInt(id, 10) + `,` + string(p) + `,` + string(e) + `);`
}

// BridgeScript defines window.vukemDesktop on the page. The shell runs it after
// every navigation ends, once the webview has set window._wails.invoke, and the
// page learns of it from the vukem-desktop-ready event (it may have rendered
// first). Dropped file paths arrive through window._wails.handlePlatformFileDrop,
// which the shell calls on every system; on Windows the webview gives the page a
// File list whose paths only the shell can read, so the page forwards a drop of
// files to it.
const BridgeScript = `(function(){var w=window;` +
	`if(w.vukemDesktop||!w._wails||typeof w._wails.invoke!=="function"){return;}` +
	`var pending={},drops=[],menus=[],next=1;` +
	`function send(op,id,path){var m={vukem:op,id:id};if(path){m.path=path;}w._wails.invoke(JSON.stringify(m));}` +
	`function ask(op,path){return new Promise(function(resolve,reject){var id=next++;pending[id]={resolve:resolve,reject:reject};send(op,id,path);});}` +
	`w._wails.vukemResolve=function(id,paths,err){var p=pending[id];if(!p){return;}delete pending[id];` +
	`if(err){p.reject(new Error(err));}else{p.resolve(paths);}};` +
	`w._wails.handlePlatformFileDrop=function(paths){drops.slice().forEach(function(cb){cb(paths.slice());});};` +
	`w._wails.vukemMenu=function(id){menus.slice().forEach(function(cb){cb(String(id));});};` +
	`var wv=w.chrome&&w.chrome.webview;` +
	`if(wv&&typeof wv.postMessageWithAdditionalObjects==="function"){` +
	`var files=function(e){return e.dataTransfer&&Array.prototype.indexOf.call(e.dataTransfer.types||[],"Files")>=0;};` +
	`document.addEventListener("dragover",function(e){if(files(e)){e.preventDefault();}},true);` +
	`document.addEventListener("drop",function(e){if(!files(e)){return;}e.preventDefault();` +
	`wv.postMessageWithAdditionalObjects("file:drop:"+Math.round(e.clientX)+":"+Math.round(e.clientY),e.dataTransfer.files);},true);}` +
	`w.vukemDesktop=Object.freeze({` +
	`pickFiles:function(){return ask("pickFiles");},` +
	`pickTemplate:function(){return ask("pickTemplate");},` +
	`showInFolder:function(path){return ask("showInFolder",path);},` +
	`onDropPaths:function(cb){drops.push(cb);return function(){var i=drops.indexOf(cb);if(i>=0){drops.splice(i,1);}};},` +
	`onMenuAction:function(cb){menus.push(cb);return function(){var i=menus.indexOf(cb);if(i>=0){menus.splice(i,1);}};}});` +
	`w.dispatchEvent(new Event("vukem-desktop-ready"));})();`
