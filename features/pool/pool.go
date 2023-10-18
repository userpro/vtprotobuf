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

var linearPoolPackage protogen.GoImportPath = protogen.GoImportPath("github.com/userpro/linearpool")

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

	p.P(`var vtprotoPool_`, ccTypeName, `Wrapper = `, p.Ident("sync", "Pool"), `{`)
	p.P(`New: func() any {`)
	p.P(`ac := `, linearPoolPackage.Ident("NewAlloctorFromPool("+p.QualifiedGoIdent(linearPoolPackage.Ident("DiKB"))+"*4"+")")) // TODO(dz) 默认内存大小设置
	p.P(`return &`, ccTypeName, `Wrapper{`)
	p.P(`ac: ac,`)
	p.P(`raw: `, linearPoolPackage.Ident("New["+ccTypeName.GoName+"](ac),"))
	p.P(`}`)
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
			p.P(`if `, tmpVarName, ` != nil {`)
			p.P(tmpVarName, `.Clear()`)
			p.P(`}`)
			// Comment: 对 map 的 message value 进行 pool 无收益
			// kind := field.Desc.Kind()
			// if (kind == protoreflect.MessageKind || kind == protoreflect.GroupKind) &&
			// 	p.ShouldPool(field.Message.Fields[1].Message) {
			// 	p.P(`for k, v := range `, tmpVarName, ` {`)
			// 	p.P(`v.ReturnToVTPool()`)
			// }
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

	p.P(`func (m *`, ccTypeName, `Wrapper) ReturnToVTPool() {`)
	p.P(`if m != nil {`)
	p.P(`m.raw.ResetVT()`)
	p.P(`vtprotoPool_`, ccTypeName, `Wrapper.Put(m)`)
	p.P(`}`)
	p.P(`}`)

	// 如果在 pb 之外使用了 alloctor 分配内存的话 需要调用该方法
	p.P(`func (m *`, ccTypeName, `Wrapper) FreeToPool() {`)
	p.P(`if m != nil {`)
	p.P(`m.ac.Reset()`)
	p.P(`m.raw = `, linearPoolPackage.Ident("New["+ccTypeName.GoName+"](m.ac)"))
	p.P(`vtprotoPool_`, ccTypeName, `Wrapper.Put(m)`)
	p.P(`}`)
	p.P(`}`)

	p.P(`func `, ccTypeName, `WrapperFromVTPool() *`, ccTypeName, `Wrapper{`)
	p.P(`return vtprotoPool_`, ccTypeName, `Wrapper.Get().(*`, ccTypeName, `Wrapper)`)
	p.P(`}`)
}
