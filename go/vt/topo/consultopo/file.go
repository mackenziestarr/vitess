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

package consultopo

import (
	"path"

	"context"

	"github.com/hashicorp/consul/api"
	"vitess.io/vitess/go/vt/topo"
)

// Create is part of the topo.Conn interface.
func (s *Server) Create(ctx context.Context, filePath string, contents []byte) (topo.Version, error) {
	nodePath := path.Join(s.root, filePath)

	// We need to do a Put with version=0 and get the version
	// back.  KV.CAS does not return that information. However, a
	// CAS in a transaction will return the node's data, so we use that.
	ops := api.KVTxnOps{
		&api.KVTxnOp{
			Verb:  api.KVCAS,
			Key:   nodePath,
			Value: contents,
			Index: 0,
		},
	}
	ok, resp, _, err := s.kv.Txn(ops, nil)
	if err != nil {
		// Communication error.
		return nil, err
	}
	if !ok {
		// Transaction was rolled back, means the node exists.
		return nil, topo.NewError(topo.NodeExists, nodePath)
	}
	return ConsulVersion(resp.Results[0].ModifyIndex), nil
}

// Update is part of the topo.Conn interface.
func (s *Server) Update(ctx context.Context, filePath string, contents []byte, version topo.Version) (topo.Version, error) {
	nodePath := path.Join(s.root, filePath)

	// Again, we need to get the final version back.
	// So we have to use a transaction, as Put doesn't return the version.
	ops := api.KVTxnOps{
		&api.KVTxnOp{
			Verb:  api.KVSet,
			Key:   nodePath,
			Value: contents,
		},
	}
	if version != nil {
		ops[0].Verb = api.KVCAS
		ops[0].Index = uint64(version.(ConsulVersion))
	}
	ok, resp, _, err := s.kv.Txn(ops, nil)
	if err != nil {
		// Communication error.
		return nil, err
	}
	if !ok {
		// Transaction was rolled back, means the node has a
		// bad version.
		return nil, topo.NewError(topo.BadVersion, nodePath)
	}
	return ConsulVersion(resp.Results[0].ModifyIndex), nil
}

// Get is part of the topo.Conn interface.
func (s *Server) Get(ctx context.Context, filePath string) ([]byte, topo.Version, error) {
	nodePath := path.Join(s.root, filePath)

	pair, _, err := s.kv.Get(nodePath, nil)
	if err != nil {
		return nil, nil, err
	}
	if pair == nil {
		return nil, nil, topo.NewError(topo.NoNode, nodePath)
	}

	return pair.Value, ConsulVersion(pair.ModifyIndex), nil
}

// Delete is part of the topo.Conn interface.
func (s *Server) Delete(ctx context.Context, filePath string, version topo.Version) error {
	nodePath := path.Join(s.root, filePath)

	// We need to differentiate if the node existed or not.
	// So we cannot use a regular Delete, which returns success
	// whether or not the node originally existed.
	// Let's do a 'Get' and then a 'Delete' in a transaction:
	// - If the node doesn't exists, the Get will fail and abort.
	// - If the node exists, the Get will work, and the Delete will
	// then execute (and may or may not work for other reasons).
	ops := api.KVTxnOps{
		&api.KVTxnOp{
			Verb: api.KVGet,
			Key:  nodePath,
		},
		&api.KVTxnOp{
			Verb: api.KVDelete,
			Key:  nodePath,
		},
	}
	if version != nil {
		// if we have a version, the delete we use specifies it.
		ops[1].Verb = api.KVDeleteCAS
		ops[1].Index = uint64(version.(ConsulVersion))
	}
	ok, resp, _, err := s.kv.Txn(ops, nil)
	if err != nil {
		// Communication error.
		return err
	}
	if !ok {
		// Transaction was rolled back, means the Get failed,
		// or the Delete had the wrong version. See which one it was.
		switch resp.Errors[0].OpIndex {
		case 0:
			// Get failed (operation 0), the node didn't exist.
			return topo.NewError(topo.NoNode, nodePath)
		case 1:
			// DeleteCAS failed (operation 1), means bad version.
			return topo.NewError(topo.BadVersion, nodePath)
		default:
			// very unexpected.
			return ErrBadResponse
		}
	}
	return nil
}
