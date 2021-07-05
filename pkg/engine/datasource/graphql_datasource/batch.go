package graphql_datasource

import (
	"bytes"
	"hash"
	"sync"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash"

	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
)

var representationPath = []string{"body", "variables", "representations"}

type batchMerger struct {
	hash64Pool sync.Pool
}

func newBatchMerger() *batchMerger {
	return &batchMerger{
		hash64Pool: sync.Pool{
			New: func() interface{} {
				return xxhash.New()
			},
		},
	}
}

func (f *batchMerger) merge(out *fastbuffer.FastBuffer, inputs ...*fastbuffer.FastBuffer) (outToInPositions map[int][]int, err error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	var variables [][]byte
	var currOutPosition int

	outToInPositions = make(map[int][]int, len(inputs))
	hashToOutPositions := make(map[uint64]int, len(inputs))

	hash64 := f.hash64Pool.Get().(hash.Hash64)
	defer f.hash64Pool.Put(hash64)

	for i := range inputs {
		inputVariables, _, _, err := jsonparser.Get(inputs[i].Bytes(), representationPath...)
		if err != nil {
			return nil, err
		}

		if _, err = hash64.Write(inputVariables); err != nil {
			return nil, err
		}
		// deduplicate inputs, do not send the same representation inputVariables
		inputHash := hash64.Sum64()
		hash64.Reset()

		if outPosition, ok := hashToOutPositions[inputHash]; ok {
			outToInPositions[outPosition] = append(outToInPositions[outPosition], i)
			continue
		}

		hashToOutPositions[inputHash] = currOutPosition
		outToInPositions[currOutPosition] = []int{i}
		currOutPosition++

		_, err = jsonparser.ArrayEach(inputVariables, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			variables = append(variables, value)
		})
		if err != nil {
			return nil, err
		}
	}

	representationJson := append([]byte("["), append(bytes.Join(variables, []byte(",")), []byte("]")...)...)

	mergedInput, err := jsonparser.Set(inputs[0].Bytes(), representationJson, representationPath...)
	if err != nil {
		return nil, err
	}

	out.WriteBytes(mergedInput)

	return outToInPositions, nil
}
