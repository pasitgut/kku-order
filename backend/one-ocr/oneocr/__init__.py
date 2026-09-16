from .common import Quad, Word, Line, OcrResult
from .engine import OneOCR

def prepare(*args, **kwargs):
    from .extractor import decrypt_and_extract as _dec
    from .downloader import download_and_extract as _dl
    final_bin = _dl(output_dir=kwargs.get("bin_dir"), target_arch=kwargs.get("target_arch", "x64"))
    return _dec(bin_dir=final_bin, models_dir=kwargs.get("models_dir"))

def decrypt_and_extract(*args, **kwargs):
    from .extractor import decrypt_and_extract as _f
    return _f(*args, **kwargs)