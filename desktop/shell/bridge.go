package shell

import (
	"encoding/json"
	"net/url"
	"regexp"
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
	// OpRevealAsset opens the folder of a stored library file (epic 94): the path is checked by the shell against
	// the media folder of its workspace, which showInFolder (an export) does not need.
	OpRevealAsset = "revealAsset"
	// The settings of the background mode (SDD-012 section 21.3 item 7).
	OpGetBackground = "getBackground"
	OpSetBackground = "setBackground"
	// The teleprompter window of a project (epic 66, SDD-025 section 20.3): the page names only the project; the
	// shell builds the address and keeps the window out of a screen capture where the system allows it.
	OpOpenTeleprompter  = "openTeleprompter"
	OpCloseTeleprompter = "closeTeleprompter"
)

// projectID is the shape of the id of a project (SDD-000 section 3): the only thing a page sends to open the
// teleprompter, so a page cannot name any other address.
var projectID = regexp.MustCompile(`^prj_[0-9a-hjkmnp-tv-z]{10}$`)

// TeleprompterURL is the address of the teleprompter page of a project on the server the shell started, or false
// when the project is not an id of a project (T-170).
func TeleprompterURL(port int, project string) (string, bool) {
	if port <= 0 || !projectID.MatchString(project) {
		return "", false
	}

	u := url.URL{
		Scheme: "http",
		Host:   "127.0.0.1:" + strconv.Itoa(port),
		Path:   "/projects/" + project + "/teleprompter",
	}

	return u.String(), true
}

// maxBridgeMessage bounds a message; a path is the longest thing it carries.
const maxBridgeMessage = 4096

// BackgroundSettings is what the page reads and writes of the background mode; it is the shape of
// desktop/service.Settings, which the shell package does not import.
type BackgroundSettings struct {
	KeepRunning   bool `json:"keep_running"`
	StartAtSignIn bool `json:"start_at_sign_in"`
	Asked         bool `json:"asked"`
}

// BridgeRequest is one message from the page. Path is set only for OpShowInFolder and OpRevealAsset, Background only
// for OpSetBackground and Project only for OpOpenTeleprompter.
type BridgeRequest struct {
	Op         string              `json:"vukem"`
	ID         int64               `json:"id"`
	Path       string              `json:"path,omitempty"`
	Background *BackgroundSettings `json:"background,omitempty"`
	Project    string              `json:"project,omitempty"`
}

// ParseBridgeMessage decodes a raw webview message. It reports false for
// anything that is not a known request, so a page cannot reach any other code.
func ParseBridgeMessage(msg string) (BridgeRequest, bool) {
	var req BridgeRequest
	if len(msg) > maxBridgeMessage || json.Unmarshal([]byte(msg), &req) != nil {
		return BridgeRequest{}, false
	}
	switch req.Op {
	case OpPickFiles, OpPickTemplate, OpGetBackground, OpCloseTeleprompter:
		req.Path, req.Background, req.Project = "", nil, ""
	case OpShowInFolder, OpRevealAsset:
		if req.Path == "" {
			return BridgeRequest{}, false
		}
		req.Background, req.Project = nil, ""
	case OpSetBackground:
		if req.Background == nil {
			return BridgeRequest{}, false
		}
		req.Path, req.Project = "", ""
	case OpOpenTeleprompter:
		if !projectID.MatchString(req.Project) {
			return BridgeRequest{}, false
		}
		req.Path, req.Background = "", nil
	default:
		return BridgeRequest{}, false
	}
	return req, true
}

// ResolveValueScript settles the promise of request id with a JSON value, or with an error message when
// errMsg is not empty. The value is marshalled here, so the page never receives text it must trust.
func ResolveValueScript(id int64, value any, errMsg string) string {
	v, err := json.Marshal(value)
	if err != nil {
		v = []byte("null")
	}
	e, err := json.Marshal(errMsg)
	if err != nil {
		e = []byte(`"error"`)
	}
	return `window._wails&&window._wails.vukemResolve&&window._wails.vukemResolve(` +
		strconv.FormatInt(id, 10) + `,` + string(v) + `,` + string(e) + `);`
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
	`function send(op,id,path,bg,project){var m={vukem:op,id:id};if(path){m.path=path;}if(bg){m.background=bg;}if(project){m.project=project;}w._wails.invoke(JSON.stringify(m));}` +
	`function ask(op,path,bg,project){return new Promise(function(resolve,reject){var id=next++;pending[id]={resolve:resolve,reject:reject};send(op,id,path,bg,project);});}` +
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
	`revealAsset:function(path){return ask("revealAsset",path);},` +
	`getBackground:function(){return ask("getBackground");},` +
	`setBackground:function(s){return ask("setBackground",undefined,s);},` +
	`openTeleprompter:function(project){return ask("openTeleprompter",undefined,undefined,project);},` +
	`closeTeleprompter:function(){return ask("closeTeleprompter");},` +
	`onDropPaths:function(cb){drops.push(cb);return function(){var i=drops.indexOf(cb);if(i>=0){drops.splice(i,1);}};},` +
	`onMenuAction:function(cb){menus.push(cb);return function(){var i=menus.indexOf(cb);if(i>=0){menus.splice(i,1);}};}});` +
	`w.dispatchEvent(new Event("vukem-desktop-ready"));})();`
