import json
import os
from concurrent import futures

import grpc

# Import the generated code
import guardian_pb2
import guardian_pb2_grpc
import numpy as np
import onnxruntime as ort

# --- CONFIG ---
MAX_LEN = 75
ONNX_MODEL_PATH = "guardian_model.onnx"
TOKENIZER_PATH = "tokenizer.json"


class CharTokenizer:
    """Lightweight char-level tokenizer that replaces the Keras Tokenizer.

    Loads from a plain JSON file containing word_index, oov_index, and flags,
    so there is zero dependency on TensorFlow/Keras at inference time.
    """

    def __init__(self, path: str):
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
        self.word_index: dict[str, int] = data["word_index"]
        self.oov_index: int = data.get("oov_index", 0)
        self.lower: bool = data.get("lower", True)

    def texts_to_sequences(self, texts: list[str]) -> list[list[int]]:
        result = []
        for text in texts:
            if self.lower:
                text = text.lower()
            seq = []
            for ch in text:
                idx = self.word_index.get(ch, self.oov_index)
                if idx != 0:
                    seq.append(idx)
            result.append(seq)
        return result


class GuardianService(guardian_pb2_grpc.GuardianAIServicer):
    def __init__(self):
        if not os.path.exists(ONNX_MODEL_PATH):
            raise FileNotFoundError(
                f"ONNX model not found at {ONNX_MODEL_PATH}. "
                "Train the model first (train_model.py) then convert it (convert_to_onnx.py)."
            )

        if not os.path.exists(TOKENIZER_PATH):
            raise FileNotFoundError(
                f"Tokenizer not found at {TOKENIZER_PATH}. "
                "Train the model first (train_model.py) or run export_tokenizer.py."
            )

        print(f"[*] Loading ONNX model from {ONNX_MODEL_PATH} ...")
        opts = ort.SessionOptions()
        opts.inter_op_num_threads = 2
        opts.intra_op_num_threads = 2
        opts.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        self.session = ort.InferenceSession(
            ONNX_MODEL_PATH, sess_options=opts, providers=["CPUExecutionProvider"]
        )
        self.input_name = self.session.get_inputs()[0].name

        print(f"[*] Loading tokenizer from {TOKENIZER_PATH} ...")
        self.tokenizer = CharTokenizer(TOKENIZER_PATH)
        print("[+] AI Ready to serve (backend: onnxruntime).")

    def _predict(self, domain):
        seq = self.tokenizer.texts_to_sequences([domain])
        padded = np.zeros((1, MAX_LEN), dtype=np.float32)
        for i, idx in enumerate(seq[0][:MAX_LEN]):
            padded[0][i] = float(idx)
        outputs = self.session.run(None, {self.input_name: padded})
        return outputs[0][0]

    def PredictDomain(self, request, context):
        domain = request.domain.lower().strip()

        prediction = self._predict(domain)

        class_idx = int(np.argmax(prediction))
        confidence = float(prediction[class_idx])

        labels = {0: "Safe", 1: "DGA", 2: "Phishing"}

        # Safety Fail-Open Logic
        if confidence < 0.60:
            class_idx = 0
            verdict = "Safe (Low Confidence)"
        else:
            verdict = labels.get(class_idx, "Unknown")

        is_malicious = bool(class_idx > 0)

        return guardian_pb2.PredictionResponse(
            is_malicious=is_malicious, confidence_score=confidence, category=verdict
        )


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    guardian_pb2_grpc.add_GuardianAIServicer_to_server(GuardianService(), server)

    server.add_insecure_port("127.0.0.1:50051")
    print("[*] gRPC Server running on 127.0.0.1:50051 ...")
    server.start()
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)


if __name__ == "__main__":
    serve()
