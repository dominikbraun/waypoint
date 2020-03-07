package plugin

import (
	"context"
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/hashicorp/go-hclog"

	"github.com/mitchellh/devflow/sdk/internal-shared/mapper"
	pb "github.com/mitchellh/devflow/sdk/proto"
)

// funcErr returns a function that can be returned for any of the
// Func component calls that just returns an error. This lets us surface
// RPC errors cleanly rather than a panic.
func funcErr(err error) interface{} {
	return func(context.Context) (interface{}, error) {
		return nil, err
	}
}

// funcToSpec takes a function pointer and generates a FuncSpec from it.
// The function must only take and return values that are proto.Message
// implementations OR have a chain of mappers that directly covert from/to a
// proto.Message.
func funcToSpec(log hclog.Logger, f interface{}, mappers []*mapper.Func) (*pb.FuncSpec, error) {
	mf, err := mapper.NewFunc(f, mapper.WithLogger(log))
	if err != nil {
		return nil, err
	}

	// We need to find a path through that only has protobuf requirements
	// or "context". These are the only given values to the func for plugins.
	types := mf.ChainInputSet(mappers, func(t mapper.Type) bool {
		rt, ok := t.(*mapper.ReflectType)
		if !ok {
			return false
		}

		typ := rt.Type
		return typ == protoMessageType ||
			typ.Implements(protoMessageType) ||
			typ == contextType
	})
	if len(types) == 0 {
		return nil, fmt.Errorf(
			"cannot satisfy the function %s. The function takes arguments that "+
				"are not proto.Messages or have no mappers to convert to proto.Messages",
			mf)
	}

	// Build our FuncSpec. The name we use is just the name on this side.
	result := pb.FuncSpec{Name: mf.Name}

	// For each type, get the Any message name for it.
	for _, t := range types {
		typ := t.(*mapper.ReflectType).Type

		// If it is context then ignore it
		if typ == contextType {
			continue
		}

		// If we're here we know its a proto.Message
		result.Args = append(result.Args, typeToMessage(t))
	}

	// Get the result type. If it isn't a proto message, we look for a chain
	// to get us to that proto message.
	out := mf.Out
	if !checkProtoMessage(out) {
		chain := mapper.ChainTarget(checkProtoMessage, mappers)
		if chain == nil {
			return nil, fmt.Errorf(
				"function must output a type that is a proto.Message or has " +
					"a chain of mappers that result in a proto.Message")
		}

		out = chain.Out()
	}
	result.Result = typeToMessage(out)

	return &result, nil
}

// specToFunc takes a FuncSpec and returns a mapper.Func that can be called
// to invoke this function.
func specToFunc(log hclog.Logger, s *pb.FuncSpec, cb interface{}) *mapper.Func {
	// Build the function
	f, err := mapper.NewFunc(cb,
		mapper.WithName(s.Name),
		mapper.WithType(dynamicArgsType, makeDynamicArgsMapperType(s)),
		mapper.WithLogger(log),
	)
	if err != nil {
		panic(err)
	}

	return f
}

// typeToMessage converts a mapper.Type to the proto.Message name value.
//
// preconditions:
//   - t is a ReflectType
//   - the typ represented by t is a proto.Message
func typeToMessage(t mapper.Type) string {
	typ := t.(*mapper.ReflectType).Type
	return proto.MessageName(reflect.Zero(typ).Interface().(proto.Message))
}

// checkProtoMessage is a mapper.CheckFunc that returns true if the type
// is a proto.Message or a struct that implements it.
func checkProtoMessage(t mapper.Type) bool {
	rt, ok := t.(*mapper.ReflectType)
	if !ok {
		return false
	}

	typ := rt.Type
	return typ == protoMessageType || typ.Implements(protoMessageType)
}

var (
	contextType      = reflect.TypeOf((*context.Context)(nil)).Elem()
	protoMessageType = reflect.TypeOf((*proto.Message)(nil)).Elem()
)