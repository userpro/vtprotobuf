package pool

import (
	"fmt"

	"github.com/planetscale/vtprotobuf/generator"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func init() {
	generator.RegisterFeature("pool", func(gen *generator.GeneratedFile) generator.FeatureGenerator {
		return &pool{GeneratedFile: gen}
	})
}

type pool struct {
	*generator.GeneratedFile
	once bool
}

var _ generator.FeatureGenerator = (*pool)(nil)

func (p *pool) GenerateHelpers() {}

func (p *pool) GenerateFile(file *protogen.File) bool {
	for _, message := range file.Messages {
		p.message(message)
	}
	return p.once
}

func (p *pool) message(message *protogen.Message) {
	for _, nested := range message.Messages {
		p.message(nested)
	}

	if message.Desc.IsMapEntry() || !p.ShouldPool(message) {
		return
	}

	p.once = true
	ccTypeName := message.GoIdent

	p.P(`var vtprotoPool_`, ccTypeName, ` = `, p.Ident("sync", "Pool"), `{`)
	p.P(`New: func() interface{} {`)
	p.P(`return &`, message.GoIdent, `{}`)
	p.P(`},`)
	p.P(`}`)

	p.P(`func (m *`, ccTypeName, `) ResetVT() {`)
	p.P(`if m == nil {`)
	p.P(`return`)
	p.P(`}`)
	var saved []*protogen.Field
	oneofFields := make(map[string][]*protogen.Field)
	for _, field := range message.Fields {
		fieldName := field.GoName

		// 仅处理非 oneof 字段
		oneof := field.Oneof != nil && !field.Oneof.Desc.IsSynthetic()
		if oneof {
			oneofFields[field.Oneof.GoName] = append(oneofFields[field.Oneof.GoName], field)
			continue
		}

		if field.Desc.IsList() {
			switch field.Desc.Kind() {
			case protoreflect.MessageKind, protoreflect.GroupKind:
				if p.ShouldPool(field.Message) {
					p.P(`for _, mm := range m.`, fieldName, `{`)
					p.P(`mm.ResetVT()`)
					p.P(`}`)
				}
				p.P(fmt.Sprintf("f%d", len(saved)), ` := m.`, fieldName, `[:0]`)
				saved = append(saved, field)
			default:
				p.P(fmt.Sprintf("f%d", len(saved)), ` := m.`, fieldName, `[:0]`)
				saved = append(saved, field)
			}
		} else if field.Desc.IsMap() {
			tmpVarName := fmt.Sprintf("f%d", len(saved))
			p.P(tmpVarName, ` := m.`, fieldName)
			// Comment: 对 map 的 message value 进行 pool 无收益
			// kind := field.Desc.Kind()
			// if (kind == protoreflect.MessageKind || kind == protoreflect.GroupKind) &&
			// 	p.ShouldPool(field.Message.Fields[1].Message) {
			// 	p.P(`for k, v := range `, tmpVarName, ` {`)
			// 	p.P(`v.ReturnToVTPool()`)
			// } else {
			p.P(`for k := range `, tmpVarName, ` {`)
			// }
			p.P(`delete(`, tmpVarName, `, k)`)
			p.P(`}`)
			saved = append(saved, field)
		} else {
			switch field.Desc.Kind() {
			case protoreflect.MessageKind, protoreflect.GroupKind:
				if p.ShouldPool(field.Message) {
					p.P(`m.`, fieldName, `.ResetVT()`)
					p.P(fmt.Sprintf("f%d", len(saved)), ` := m.`, fieldName)
					saved = append(saved, field)
				}
			}
		}
	}

	// 处理 oneof字段
	oneofSaved := []string{}
	for fieldOneofName, fields := range oneofFields {
		needGen := false
		for _, field := range fields {
			switch field.Desc.Kind() {
			case protoreflect.MessageKind, protoreflect.GroupKind:
				if p.ShouldPool(field.Message) {
					needGen = true
				}
			}
		}
		if !needGen {
			continue
		}

		p.P(fmt.Sprintf("f%d", len(oneofSaved)+len(saved)), ` := m.`, fieldOneofName)
		oneofSaved = append(oneofSaved, fieldOneofName)
		p.P(`switch v := m.`, fieldOneofName, `.(type) {`)
		for _, field := range fields {
			fieldName := field.GoName

			switch field.Desc.Kind() {
			case protoreflect.MessageKind, protoreflect.GroupKind:
				if p.ShouldPool(field.Message) {
					p.P(`case *`, field.GoIdent, `:`)
					p.P(`v.`, fieldName, `.ResetVT()`)
				}
			}
		}
		p.P(`}`)
	}

	// p.P(`m.Reset()`)
	p.P(`*m = `, ccTypeName, `{}`)
	for i, field := range saved {
		p.P(`m.`, field.GoName, ` = `, fmt.Sprintf("f%d", i))
	}

	for i, field := range oneofSaved {
		p.P(`m.`, field, ` = `, fmt.Sprintf("f%d", i+len(saved)))
	}
	p.P(`}`)

	p.P(`func (m *`, ccTypeName, `) ReturnToVTPool() {`)
	p.P(`if m != nil {`)
	p.P(`m.ResetVT()`)
	p.P(`vtprotoPool_`, ccTypeName, `.Put(m)`)
	p.P(`}`)
	p.P(`}`)

	p.P(`func `, ccTypeName, `FromVTPool() *`, ccTypeName, `{`)
	p.P(`return vtprotoPool_`, ccTypeName, `.Get().(*`, ccTypeName, `)`)
	p.P(`}`)
}
