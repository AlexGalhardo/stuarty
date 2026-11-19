A workspace written by the binary of release v0.29.0 (the last one named vukem-video), with the fixed
scenario of service/testdata/upgrade/build-workspace.sh: a project with a clip, captions and keyframes, a
template, a trashed and restored project and one export. The token was removed (the server writes a new one),
and so were the logs and the scenario files.

Rebuilt with:  git worktree add --detach <dir> v0.29.0
               go build -o vukem-video ./web/cmd/app           (inside <dir>)
               bash service/testdata/upgrade/build-workspace.sh <binary> <new workspace> <clip.mp4>

Tests copy it to a temporary home as .vukem-video and start the current binary on it (web/cmd/app/adopt_test.go,
service/workspace/adopt_test.go). Never edit it.
