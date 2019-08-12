userConfigPlugin.so: *.go
	GO111MODULE=on CGO_ENABLED=1 go build -race -gcflags '' -ldflags '-X "github.com/pingcap/pd/server.PDReleaseVersion=v4.0.0-alpha-18-g0dd6faf-dirty" -X "github.com/pingcap/pd/server.PDBuildTS=2019-07-31 06:45:40" -X "github.com/pingcap/pd/server.PDGitHash=0dd6faf326fc78c05e9c74cf58318bc6bebdbb35" -X "github.com/pingcap/pd/server.PDGitBranch=master"' -buildmode=plugin -o userConfigPlugin.so *.go

.PHONY : clean

clean:
	rm userConfigPlugin.so
