// Copyright (c) 2021 PlanetScale Inc. All rights reserved.
// Copyright (c) 2013, The GoGo Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unmarshal

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/planetscale/vtprotobuf/generator"
)

func (p *unmarshal) messageUnsafe(proto3 bool, message *protogen.Message) {
	for _, nested := range message.Messages {
		p.messageUnsafe(proto3, nested)
	}

	if message.Desc.IsMapEntry() {
		return
	}

	p.once = true
	ccTypeName := message.GoIdent
	required := message.Desc.RequiredNumbers()

	p.P(`func (m *`, ccTypeName, `) UnmarshalVTUnSafe(dAtA []byte) error {`)
	if required.Len() > 0 {
		p.P(`var hasFields [`, strconv.Itoa(1+(required.Len()-1)/64), `]uint64`)
	}
	p.P(`l := len(dAtA)`)
	p.P(`iNdEx := 0`)
	p.P(`for iNdEx < l {`)
	p.P(`preIndex := iNdEx`)
	p.P(`var wire uint64`)
	p.decodeVarint("wire", "uint64")
	p.P(`fieldNum := int32(wire >> 3)`)
	p.P(`wireType := int(wire & 0x7)`)
	p.P(`if wireType == `, strconv.Itoa(int(protowire.EndGroupType)), ` {`)
	p.P(`return `, p.Ident("fmt", "Errorf"), `("proto: `, message.GoIdent.GoName, `: wiretype end group for non-group")`)
	p.P(`}`)
	p.P(`if fieldNum <= 0 {`)
	p.P(`return `, p.Ident("fmt", "Errorf"), `("proto: `, message.GoIdent.GoName, `: illegal tag %d (wire type %d)", fieldNum, wire)`)
	p.P(`}`)
	p.P(`switch fieldNum {`)
	for _, field := range message.Fields {
		p.field(proto3, false, field, message, required, true)
	}
	p.P(`default:`)
	p.P(`iNdEx=preIndex`)
	p.P(`skippy, err := skip(dAtA[iNdEx:])`)
	p.P(`if err != nil {`)
	p.P(`return err`)
	p.P(`}`)
	p.P(`if (skippy < 0) || (iNdEx + skippy) < 0 {`)
	p.P(`return ErrInvalidLength`)
	p.P(`}`)
	p.P(`if (iNdEx + skippy) > l {`)
	p.P(`return `, p.Ident("io", `ErrUnexpectedEOF`))
	p.P(`}`)
	if message.Desc.ExtensionRanges().Len() > 0 {
		c := []string{}
		eranges := message.Desc.ExtensionRanges()
		for e := 0; e < eranges.Len(); e++ {
			erange := eranges.Get(e)
			c = append(c, `((fieldNum >= `+strconv.Itoa(int(erange[0]))+`) && (fieldNum < `+strconv.Itoa(int(erange[1]))+`))`)
		}
		p.P(`if `, strings.Join(c, "||"), `{`)
		p.P(`err = `, p.Ident(generator.ProtoPkg, "UnmarshalOptions"), `{AllowPartial: true}.Unmarshal(dAtA[iNdEx:iNdEx+skippy], m)`)
		p.P(`if err != nil {`)
		p.P(`return err`)
		p.P(`}`)
		p.P(`iNdEx += skippy`)
		p.P(`} else {`)
	}
	p.P(`m.unknownFields = append(m.unknownFields, dAtA[iNdEx:iNdEx+skippy]...)`)
	p.P(`iNdEx += skippy`)
	if message.Desc.ExtensionRanges().Len() > 0 {
		p.P(`}`)
	}
	p.P(`}`)
	p.P(`}`)

	for _, field := range message.Fields {
		if field.Desc.Cardinality() != protoreflect.Required {
			continue
		}
		var fieldBit int
		for fieldBit = 0; fieldBit < required.Len(); fieldBit++ {
			if required.Get(fieldBit) == field.Desc.Number() {
				break
			}
		}
		if fieldBit == required.Len() {
			panic("missing required field")
		}
		p.P(`if hasFields[`, strconv.Itoa(int(fieldBit/64)), `] & uint64(`, fmt.Sprintf("0x%08x", uint64(1)<<(fieldBit%64)), `) == 0 {`)
		p.P(`return `, p.Ident("fmt", "Errorf"), `("proto: required field `, field.Desc.Name(), ` not set")`)
		p.P(`}`)
	}
	p.P()
	p.P(`if iNdEx > l {`)
	p.P(`return `, p.Ident("io", `ErrUnexpectedEOF`))
	p.P(`}`)
	p.P(`return nil`)
	p.P(`}`)
}
