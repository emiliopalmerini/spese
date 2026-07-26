class PCMCaptureProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.ratio = sampleRate / 16000;
    this.position = 0;
    this.previous = 0;
    this.pending = [];
    this.port.onmessage = function (event) {
      if (event.data === 'flush') {
        this.flush();
      }
    }.bind(this);
  }

  flush() {
    if (this.pending.length === 0) {
      return;
    }
    var pcm = new Int16Array(this.pending.length);
    for (var i = 0; i < this.pending.length; i += 1) {
      var value = this.pending[i];
      pcm[i] = value < 0 ? value * 0x8000 : value * 0x7fff;
    }
    this.pending = [];
    this.port.postMessage(pcm.buffer, [pcm.buffer]);
  }

  process(inputs) {
    var input = inputs[0] && inputs[0][0];
    if (!input || input.length === 0) {
      return true;
    }

    var source = new Float32Array(input.length + 1);
    source[0] = this.previous;
    source.set(input, 1);
    while (this.position < source.length - 1) {
      var left = Math.floor(this.position);
      var fraction = this.position - left;
      var sample = source[left] + ((source[left + 1] - source[left]) * fraction);
      this.pending.push(Math.max(-1, Math.min(1, sample)));
      this.position += this.ratio;
    }
    this.position -= source.length - 1;
    this.previous = source[source.length - 1];

    if (this.pending.length >= 2048) {
      this.flush();
    }
    return true;
  }
}

registerProcessor('spese-pcm-capture', PCMCaptureProcessor);
