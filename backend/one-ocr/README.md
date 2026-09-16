# one-ocr

Cross-platform OCR — extracted from Windows 11 OneOCR engine.

Runs on **Windows / Linux / WSL / Docker** — no Wine, no DLL at runtime.

## What is this

Microsoft ships OneOCR inside Snipping Tool (`oneocr.dll` + `oneocr.onemodel` encrypted). This package decrypts the container, extracts 11 ONNX sub-models + 9 language vocabularies, then runs inference via ONNX Runtime directly.

**Supported:** Thai, Latin, Cyrillic, CJK, Arabic, Devanagari, Hebrew, Bengali, Greek

## Structure

```
one-ocr/
├── oneocr/                              # cross-platform package
│   ├── engine.py                        # OneOCR orchestrator
│   ├── detector.py                      # text region detection (FPN)
│   ├── classifier.py                    # script classification
│   ├── recognizer.py                    # CTC recognition
│   ├── extractor.py                     # model extraction (Win only, FUNCTYPE fixed)
│   └── common.py                        # data classes
├── bin/                                 # [gitignored] source binaries (Windows only)
│   ├── oneocr.dll                       # OneOCR engine DLL (42 MB)
│   ├── oneocr.onemodel                  # encrypted model container (58 MB)
│   └── onnxruntime.dll                  # ONNX Runtime (13 MB)
├── models/                              # [gitignored] extracted ONNX models
│   ├── detector/
│   │   └── text_detector.onnx           # FPN text detector (11 MB)
│   ├── classifier/
│   │   └── script_classifier.onnx       # script/language classifier (3 MB)
│   ├── recognizers/
│   │   ├── recognizer_cjk.onnx          # Chinese/Japanese/Korean (13 MB)
│   │   ├── recognizer_latin.onnx        # Latin/European (6 MB)
│   │   ├── recognizer_cyrillic.onnx     # Cyrillic (2 MB)
│   │   ├── recognizer_arabic.onnx       # Arabic (3 MB)
│   │   ├── recognizer_devanagari.onnx   # Hindi/Sanskrit (3 MB)
│   │   ├── recognizer_hebrew.onnx       # Hebrew (3 MB)
│   │   ├── recognizer_thai.onnx         # Thai (3 MB)
│   │   ├── recognizer_bengali.onnx      # Bengali (1.7 MB)
│   │   └── recognizer_greek.onnx        # Greek (1.9 MB)
│   └── vocab/
│       ├── vocab_cjk.txt                # 32632 entries
│       ├── vocab_latin.txt              # 415 entries
│       ├── vocab_cyrillic.txt           # 548 entries
│       ├── vocab_arabic.txt             # 221 entries
│       ├── vocab_devanagari.txt         # 237 entries
│       ├── vocab_hebrew.txt             # 244 entries
│       ├── vocab_thai.txt               # 199 entries
│       ├── vocab_bengali.txt            # 201 entries
│       └── vocab_greek.txt              # 179 entries
├── example.py                           # CLI
├── pyproject.toml
└── README.md
```

## Install

```bash
git clone https://github.com/pasitgut/one-ocr.git
cd one-ocr
python -m venv .venv
source .venv/bin/activate   # Linux/WSL
# .venv\Scripts\activate    # Windows
pip install -e .
```

## Get source files (Windows only, one-time)

Find Snipping Tool installation path:

```powershell
Get-AppxPackage Microsoft.ScreenSketch | Select-Object -ExpandProperty InstallLocation
```

Example output:
```
C:\Program Files\WindowsApps\Microsoft.ScreenSketch_11.2507.15.0_x64__8wekyb3d8bbwe
```

Copy 3 files from `SnippingTool` subfolder to `bin/`:

```powershell
$installPath = Get-AppxPackage Microsoft.ScreenSketch | Select-Object -ExpandProperty InstallLocation
Copy-Item "$installPath\SnippingTool\oneocr.dll" .\bin\
Copy-Item "$installPath\SnippingTool\oneocr.onemodel" .\bin\
Copy-Item "$installPath\SnippingTool\onnxruntime.dll" .\bin\
```

Or manually copy these 3 files to `bin/`:
- `oneocr.dll` (42 MB) — OCR engine
- `oneocr.onemodel` (58 MB) — encrypted model container
- `onnxruntime.dll` (13 MB) — ONNX Runtime

## Extract models (Windows only, one-time)

Requires `bin/` with `oneocr.dll` + `oneocr.onemodel` + `onnxruntime.dll`:

```bash
python -c "from oneocr import decrypt_and_extract; decrypt_and_extract('bin','models')"
```

Output: `models/detector/`, `models/classifier/`, `models/recognizers/`, `models/vocab/`

Then copy `models/` to Linux/WSL — DLLs no longer needed.

## Usage

### Python API

```python
from pathlib import Path
from PIL import Image
from oneocr import OneOCR

ocr = OneOCR(config_dir=Path("models"), default_rotation=0)
result = ocr.recognize(Image.open("image.png"), language="thai")
print(result.full_text)
for line in result.lines:
    print(line.text, line.bbox)
```

### CLI

```bash
python example.py image.png                    # auto-detect language
python example.py image.png --lang thai        # force language
python example.py image.png -o output.txt      # save to file
python -m oneocr image.png --lang thai
```

## Language options

| --lang | Script |
|---|---|
| thai | Thai |
| latin | English/European |
| cjk | Chinese/Japanese/Korean |
| cyrillic | Russian/Slavic |
| arabic | Arabic |
| devanagari | Hindi/Sanskrit |
| hebrew | Hebrew |
| bengali | Bengali |
| greek | Greek |

Without `--lang`, auto-detects with fallback (Thai -> Latin).

## Performance

- Detector: ~0.3s
- Classifier: ~0.1s per crop
- Recognizer: ~0.2s per line (CTC, CPU)
- Total: ~2-3s per page (CPU, no GPU)
- DLL original: 0.17s (Windows, CPU only)

## How it works

1. `oneocr.onemodel` contains 11 ONNX models encrypted with key
2. `extractor.py` hooks ONNX Runtime C API to capture decrypted model bytes
3. Models saved to `models/` — standard ONNX, no encryption
4. `engine.py` runs detector -> classifier -> recognizer pipeline
5. Output: text + bounding boxes + word confidence + page angle

## Note

- `bin/` directory NOT committed (Microsoft proprietary)
- `models/` directory NOT committed (extracted from Microsoft binaries)
- Model weights are Microsoft intellectual property
- For educational and research purposes
- Reverse engineering permitted under applicable laws
