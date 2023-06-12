package elf

import (
	"debug/elf"
	"errors"
	"fmt"

	gosym2 "github.com/grafana/agent/component/pyroscope/ebpf/ebpfspy/symtab/gosym"
)

type GoTable struct {
	Index          gosym2.FlatFuncIndex
	File           *MMapedElfFile
	gopclnSection  *elf.SectionHeader
	funcNameOffset uint64
}

func (e *GoTable) Resolve(addr uint64) string {
	n := len(e.Index.Name)
	if n == 0 {
		return ""
	}
	if addr >= e.Index.End {
		return ""
	}
	i := e.Index.Entry.FindIndex(addr)
	if i == -1 {
		return ""
	}
	name, _ := e.goSymbolName(i)
	return name
}

func (e *GoTable) Cleanup() {
	e.File.Close()
}

var (
	errEmptyText         = errors.New("empty .text")
	errEmptyGoPCLNTab    = errors.New("empty .gopclntab")
	errGoTooOld          = errors.New("gosymtab: go sym tab too old")
	errGoParseFailed     = errors.New("gosymtab: go sym tab parse failed")
	errGoFailed          = errors.New("gosymtab: go sym tab  failed")
	errGoOOB             = fmt.Errorf("go table oob")
	errGoSymbolsNotFound = errors.New("gosymtab: no go symbols found")
)

func (f *MMapedElfFile) NewGoTable() (*GoTable, error) {
	obj := f
	var err error
	var pclntabData []byte
	text := obj.Section(".text")
	if text == nil {
		return nil, errEmptyText
	}
	pclntab := obj.Section(".gopclntab")
	if pclntab == nil {
		return nil, errEmptyGoPCLNTab
	}

	if pclntabData, err = obj.SectionData(pclntab); err != nil {
		return nil, err
	}

	textStart := gosym2.ParseRuntimeTextFromPclntab18(pclntabData)

	if textStart == 0 {
		// for older versions text.Addr is enough
		// https://github.com/golang/go/commit/b38ab0ac5f78ac03a38052018ff629c03e36b864
		textStart = text.Addr
	}
	if textStart < text.Addr || textStart >= text.Addr+text.Size {
		return nil, fmt.Errorf(" runtime.text out of .text bounds %d %d %d", textStart, text.Addr, text.Size)
	}
	pcln := gosym2.NewLineTable(pclntabData, textStart)

	if !pcln.IsGo12() {
		return nil, errGoTooOld
	}
	if pcln.IsFailed() {
		return nil, errGoParseFailed
	}
	funcs := pcln.Go12Funcs()
	if len(funcs.Name) == 0 || funcs.Entry.Length() == 0 || funcs.End == 0 {
		return nil, errGoSymbolsNotFound
	}
	//if funcs.Entry32 == nil && funcs.Entry64 == nil {
	//	return nil, errGoParseFailed // this should not happen
	//}
	//if funcs.Entry32 != nil && funcs.Entry64 != nil {
	//	return nil, errGoParseFailed // this should not happen
	//}
	if funcs.Entry.Length() != len(funcs.Name) {
		return nil, errGoParseFailed // this should not happen
	}

	funcNameOffset := pcln.FuncNameOffset()
	return &GoTable{
		Index:          funcs,
		File:           f,
		gopclnSection:  pclntab,
		funcNameOffset: funcNameOffset,
	}, nil
}

func (f *GoTable) goSymbolName(idx int) (string, error) {
	gopclndata, err := f.File.SectionData(f.gopclnSection)
	if err != nil {
		return "", err
	}
	if int(f.funcNameOffset) >= len(gopclndata) {
		return "", errGoOOB
	}
	funcnamedata := gopclndata[f.funcNameOffset:]
	if idx >= len(f.Index.Name) {
		return "", errGoOOB
	}
	nameOffset := f.Index.Name[idx]
	name, ok := getString(funcnamedata, int(nameOffset))
	if !ok {
		return "", errGoFailed
	}
	return name, nil
}

type GoTableWithFallback struct {
	GoTable  *GoTable
	SymTable *SymbolTable
}

func (g *GoTableWithFallback) Resolve(addr uint64) string {
	name := g.GoTable.Resolve(addr)
	if name != "" {
		return name
	}
	return g.SymTable.Resolve(addr)
}

func (g *GoTableWithFallback) Cleanup() {
	g.GoTable.Cleanup()
	g.SymTable.Cleanup() // second call is no op now, but call anyway just in case
}