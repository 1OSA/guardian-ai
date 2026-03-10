import os
import sys
import tempfile

import numpy as np

try:
    import tensorflow as tf
except ImportError:
    print("Error: tensorflow is required to convert the model.")
    print("Install it with: pip install tensorflow")
    sys.exit(1)

try:
    import tf2onnx
except ImportError:
    print("Error: tf2onnx is required to convert the model.")
    print("Install it with: pip install tf2onnx")
    sys.exit(1)

H5_PATH = "guardian_model.h5"
ONNX_PATH = "guardian_model.onnx"
MAX_LEN = 75


def convert():
    print(f"Loading Keras model from {H5_PATH} ...")
    model = tf.keras.models.load_model(H5_PATH)
    model.summary()

    # Export to SavedModel first to avoid Keras 3 / tf2onnx compatibility issues.
    saved_model_dir = tempfile.mkdtemp(prefix="guardian-savedmodel-")
    print(f"Exporting to SavedModel at {saved_model_dir} ...")
    model.export(saved_model_dir)

    print("Converting SavedModel to ONNX ...")
    import subprocess

    result = subprocess.run(
        [
            sys.executable,
            "-m",
            "tf2onnx.convert",
            "--saved-model",
            saved_model_dir,
            "--output",
            ONNX_PATH,
            "--opset",
            "15",
        ],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0:
        print("tf2onnx stderr:")
        print(result.stderr)
        print("tf2onnx stdout:")
        print(result.stdout)
        sys.exit(1)

    size_kb = os.path.getsize(ONNX_PATH) / 1024
    print(f"Saved {ONNX_PATH} ({size_kb:.0f} KB)")

    # Sanity check: run a prediction through the ONNX model.
    print("Running sanity check ...")
    try:
        import onnxruntime as ort
    except ImportError:
        print("onnxruntime not installed, skipping sanity check.")
        print("Done.")
        return

    session = ort.InferenceSession(ONNX_PATH)
    input_name = session.get_inputs()[0].name

    dummy = np.zeros((1, MAX_LEN), dtype=np.float32)
    result = session.run(None, {input_name: dummy})

    labels = ["Safe", "DGA", "Phishing"]
    pred = int(np.argmax(result[0][0]))
    conf = float(result[0][0][pred])
    print(f"Dummy input -> {labels[pred]} ({conf * 100:.1f}%)")
    print("Done.")


if __name__ == "__main__":
    convert()
