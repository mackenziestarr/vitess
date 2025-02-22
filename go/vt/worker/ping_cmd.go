/*
Copyright 2019 The Vitess Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package worker

import (
	"flag"
	"html/template"
	"net/http"

	"vitess.io/vitess/go/vt/proto/vtrpc"
	"vitess.io/vitess/go/vt/vterrors"

	"context"
	"vitess.io/vitess/go/vt/wrangler"
)

const pingHTML = `
<!DOCTYPE html>
<head>
  <title>Ping Action</title>
</head>
<body>
  <h1>Ping Action</h1>

    {{if .Error}}
      <b>Error:</b> {{.Error}}</br>
    {{else}}
	    <form action="/Debugging/Ping" method="post">
	      <LABEL for="message">Message to be logged with level CONSOLE: </LABEL>
					<INPUT type="text" id="message" name="message" value="pong"></BR>
	      <INPUT type="submit" value="Ping"/>
    </form>
    {{end}}
</body>
`

var pingTemplate = mustParseTemplate("ping", pingHTML)

func commandPing(wi *Instance, wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (Worker, error) {
	if err := subFlags.Parse(args); err != nil {
		return nil, err
	}
	if subFlags.NArg() != 1 {
		subFlags.Usage()
		return nil, vterrors.Errorf(vtrpc.Code_INVALID_ARGUMENT, "command Ping requires <message>")
	}
	message := subFlags.Arg(0)

	worker, err := NewPingWorker(wr, message)
	if err != nil {
		return nil, vterrors.Wrap(err, "Could not create Ping worker")
	}
	return worker, nil
}

func interactivePing(ctx context.Context, wi *Instance, wr *wrangler.Wrangler, w http.ResponseWriter, r *http.Request) (Worker, *template.Template, map[string]interface{}, error) {
	if err := r.ParseForm(); err != nil {
		return nil, nil, nil, vterrors.Wrap(err, "Cannot parse form")
	}

	message := r.FormValue("message")
	if message == "" {
		result := make(map[string]interface{})
		return nil, pingTemplate, result, nil
	}

	wrk, err := NewPingWorker(wr, message)
	if err != nil {
		return nil, nil, nil, vterrors.Wrap(err, "Could not create Ping worker")
	}
	return wrk, nil, nil, nil
}

func init() {
	AddCommand("Debugging", Command{"Ping",
		commandPing, interactivePing,
		"<message>",
		"For internal tests only. <message> will be logged as CONSOLE output."})
}
