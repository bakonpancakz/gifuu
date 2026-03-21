package tools

import (
	"context"
	"gifuu/include"
	"sync"
	"time"

	onnx "github.com/yalue/onnxruntime_go"
)

const (
	MODEL_THRESHOLD_DENY = 0.95
	MODEL_THRESHOLD_HIDE = 0.85
	MODEL_SIZE           = 224
	MODEL_FRAMERATE      = 3
)

var onnxSession *onnx.DynamicAdvancedSession

type ClassifyResults struct {
	Drawing float32
	Hentai  float32
	Neutral float32
	Porn    float32
	Sexy    float32
}

func SetupModel(stop context.Context, await *sync.WaitGroup) {
	if ONNX_RUNTIME_PATH == "" {
		LoggerModel.Log(WARN, "Set runtime path with envvar ONNX_RUNTIME_PATH to enable model")
		return
	}
	t := time.Now()

	// Initialize Environment
	onnx.SetSharedLibraryPath(ONNX_RUNTIME_PATH)
	if err := onnx.InitializeEnvironment(); err != nil {
		LoggerModel.Log(FATAL, "Failed to initialize ONNX Runtime: %s", err)
	}

	// Initialize Settings
	options, err := onnx.NewSessionOptions()
	if err != nil {
		LoggerModel.Log(FATAL, "Failed to create session options: %s", err)
	}
	defer options.Destroy()

	if ONNX_RUNTIME_CUDA {
		cudaOptions, err := onnx.NewCUDAProviderOptions()
		if err != nil {
			LoggerModel.Log(WARN, "CUDA unavailable, falling back to CPU: %s", err)
		} else {
			defer cudaOptions.Destroy()
			cudaOptions.Update(map[string]string{
				"cudnn_conv_algo_search": "DEFAULT", // use the only working frontend
			})
			options.AppendExecutionProviderCUDA(cudaOptions)
		}
	}

	// Initialize Model
	session, err := onnx.NewDynamicAdvancedSessionWithONNXData(
		include.NSFW_MODEL,
		[]string{"input"},
		[]string{"prediction"},
		options,
	)
	if err != nil {
		LoggerModel.Log(FATAL, "Failed to load model: %s", err)
	}
	onnxSession = session

	// Test Model with Dummy Data
	dummy := make([]float32, MODEL_SIZE*MODEL_SIZE*3)
	if _, err := ModelClassifyTensorBatch(dummy, 1); err != nil {
		LoggerModel.Log(FATAL, "Failed to initialize model: %s", err)
	}

	await.Add(1)
	go func() {
		defer await.Done()
		<-stop.Done()
		onnxSession.Destroy()
		onnxSession = nil
		onnx.DestroyEnvironment()
		LoggerModel.Log(INFO, "Closed")
	}()

	LoggerModel.Log(INFO, "Model ready in %s", time.Since(t))
}

func ModelClassifyTensorBatch(data []float32, count int) ([]ClassifyResults, error) {

	// Model is disabled, generate some dummy results.
	if onnxSession == nil {
		results := make([]ClassifyResults, count)
		for i := 0; i < count; i++ {
			results = append(results, ClassifyResults{Neutral: 1})
		}
		return results, nil
	}

	inputTensor, err := onnx.NewTensor(
		onnx.NewShape(int64(count), MODEL_SIZE, MODEL_SIZE, 3),
		data,
	)
	if err != nil {
		return nil, err
	}
	defer inputTensor.Destroy()

	outputs := []onnx.ArbitraryTensor{nil}
	if err := onnxSession.Run([]onnx.ArbitraryTensor{inputTensor}, outputs); err != nil {
		return nil, err
	}
	defer outputs[0].Destroy()

	raw := outputs[0].(*onnx.Tensor[float32]).GetData()
	results := make([]ClassifyResults, count)

	for i := range results {
		base := i * 5
		results[i] = ClassifyResults{
			Drawing: raw[base+0],
			Hentai:  raw[base+1],
			Neutral: raw[base+2],
			Porn:    raw[base+3],
			Sexy:    raw[base+4],
		}
	}

	return results, nil
}
