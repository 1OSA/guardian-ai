import pickle
import time
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
H5_MODEL_PATH = "guardian_model.h5"
TOKENIZER_PATH = "tokenizer.pickle"


class GuardianService(guardian_pb2_grpc.GuardianAIServicer):
    def __init__(self):
        import os

        # Prefer ONNX model, fall back to H5 if not found.
        if os.path.exists(ONNX_MODEL_PATH):
            print(f"[*] Loading ONNX model from {ONNX_MODEL_PATH} ...")
            opts = ort.SessionOptions()
            opts.inter_op_num_threads = 2
            opts.intra_op_num_threads = 2
            opts.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
            self.session = ort.InferenceSession(
                ONNX_MODEL_PATH, sess_options=opts, providers=["CPUExecutionProvider"]
            )
            self.input_name = self.session.get_inputs()[0].name
            self.backend = "onnx"
        elif os.path.exists(H5_MODEL_PATH):
            print(
                f"[*] ONNX model not found, falling back to TensorFlow ({H5_MODEL_PATH}) ..."
            )
            import tensorflow as tf

            tf.get_logger().setLevel("ERROR")
            self.tf_model = tf.keras.models.load_model(H5_MODEL_PATH)
            self.pad_sequences = tf.keras.preprocessing.sequence.pad_sequences
            self.backend = "tensorflow"
        else:
            raise FileNotFoundError(
                f"No model file found. Expected {ONNX_MODEL_PATH} or {H5_MODEL_PATH}"
            )

        print("[*] Loading Tokenizer ...")
        with open(TOKENIZER_PATH, "rb") as f:
            self.tokenizer = pickle.load(f)
        print(f"[+] AI Ready to serve (backend: {self.backend}).")

    def _predict(self, domain):
        seq = self.tokenizer.texts_to_sequences([domain])

        if self.backend == "onnx":
            padded = np.zeros((1, MAX_LEN), dtype=np.float32)
            for i, idx in enumerate(seq[0][:MAX_LEN]):
                padded[0][i] = float(idx)
            outputs = self.session.run(None, {self.input_name: padded})
            return outputs[0][0]
        else:
            padded = self.pad_sequences(seq, maxlen=MAX_LEN, padding="post")
            prediction = self.tf_model.predict(padded, verbose=0)
            return prediction[0]

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

    server.add_insecure_port("[::]:50051")
    print("[*] gRPC Server running on port 50051 ...")
    server.start()
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)


if __name__ == "__main__":
    serve()
