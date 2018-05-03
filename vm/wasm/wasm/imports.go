// Copyright 2017~2022 The Bottos Authors
// This file is part of the Bottos Chain library.
// Created by Rocket Core Team of Bottos.

// This program is free software: you can distribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with Bottos.  If not, see <http://www.gnu.org/licenses/>.

// Copyright 2017 The go-interpreter Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wasm

import (
	"errors"
	"fmt"
)

// Import is an intreface implemented by types that can be imported by a WebAssembly module.
type Import interface {
	isImport()
}

// ImportEntry describes an import statement in a Wasm module.
type ImportEntry struct {
	ModuleName string // module name string
	FieldName  string // field name string
	Kind       External

	// If Kind is Function, Type is a FuncImport containing the type index of the function signature
	// If Kind is Table, Type is a TableImport containing the type of the imported table
	// If Kind is Memory, Type is a MemoryImport containing the type of the imported memory
	// If the Kind is Global, Type is a GlobalVarImport
	Type Import
}

type FuncImport struct {
	Type uint32
}

func (FuncImport) isImport() {}

type TableImport struct {
	Type Table
}

func (TableImport) isImport() {}

type MemoryImport struct {
	Type Memory
}

func (MemoryImport) isImport() {}

type GlobalVarImport struct {
	Type GlobalVar
}

func (GlobalVarImport) isImport() {}

var (
	ErrImportMutGlobal           = errors.New("wasm: cannot import global mutable variable")
	ErrNoExportsInImportedModule = errors.New("wasm: imported module has no exports")
)

type InvalidExternalError uint8

func (e InvalidExternalError) Error() string {
	return fmt.Sprintf("wasm: invalid external_kind value %d", uint8(e))
}

type ExportNotFoundError struct {
	ModuleName string
	FieldName  string
}

type KindMismatchError struct {
	ModuleName string
	FieldName  string
	Import     External
	Export     External
}

func (e KindMismatchError) Error() string {
	return fmt.Sprintf("wasm: Mismatching import and export external kind values for %s.%s (%v, %v)", e.FieldName, e.ModuleName, e.Import, e.Export)
}

func (e ExportNotFoundError) Error() string {
	return fmt.Sprintf("wasm: couldn't find export with name %s in module %s", e.FieldName, e.ModuleName)
}

type InvalidFunctionIndexError uint32

func (e InvalidFunctionIndexError) Error() string {
	return fmt.Sprintf("wasm: Invalid index to function index space: %#x", uint32(e))
}

func (module *Module) resolveImports(resolve ResolveFunc) error {
	if module.Import == nil {
		return nil
	}

	modules := make(map[string]*Module)

	var funcs uint32
	for _, importEntry := range module.Import.Entries {
		importedModule, ok := modules[importEntry.ModuleName]
		if !ok {
			var err error
			importedModule, err = resolve(importEntry.ModuleName)
			if err != nil {
				return err
			}

			modules[importEntry.ModuleName] = importedModule
		}

		if importedModule.Export == nil {
			return ErrNoExportsInImportedModule
		}

		exportEntry, ok := importedModule.Export.Entries[importEntry.FieldName]
		if !ok {
			return ExportNotFoundError{importEntry.ModuleName, importEntry.FieldName}
		}

		if exportEntry.Kind != importEntry.Kind {
			return KindMismatchError{
				FieldName:  importEntry.FieldName,
				ModuleName: importEntry.ModuleName,
				Import:     importEntry.Kind,
				Export:     exportEntry.Kind,
			}
		}

		index := exportEntry.Index

		switch exportEntry.Kind {
		case ExternalFunction:
			fn := importedModule.GetFunction(int(index))
			if fn == nil {
				return InvalidFunctionIndexError(index)
			}
			module.FunctionIndexSpace = append(module.FunctionIndexSpace, *fn)
			module.Code.Bodies = append(module.Code.Bodies, *fn.Body)
			module.imports.Funcs = append(module.imports.Funcs, funcs)
			funcs++
		case ExternalGlobal:
			glb := importedModule.GetGlobal(int(index))
			if glb == nil {
				return InvalidGlobalIndexError(index)
			}
			if glb.Type.Mutable {
				return ErrImportMutGlobal
			}
			module.GlobalIndexSpace = append(module.GlobalIndexSpace, *glb)
			module.imports.Globals++

			// In both cases below, index should be always 0 (according to the MVP)
			// We check it against the length of the index space anyway.
		case ExternalTable:
			if int(index) >= len(importedModule.TableIndexSpace) {
				return InvalidTableIndexError(index)
			}
			module.TableIndexSpace[0] = importedModule.TableIndexSpace[0]
			module.imports.Tables++
		case ExternalMemory:
			if int(index) >= len(importedModule.LinearMemoryIndexSpace) {
				return InvalidLinearMemoryIndexError(index)
			}
			module.LinearMemoryIndexSpace[0] = importedModule.LinearMemoryIndexSpace[0]
			module.imports.Memories++
		default:
			return InvalidExternalError(exportEntry.Kind)
		}
	}
	return nil
}
