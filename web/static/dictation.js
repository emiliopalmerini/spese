(function () {
  function initialize(root) {
    if (!root || root.dataset.dictationInitialized === 'true') {
      return;
    }
    root.dataset.dictationInitialized = 'true';

    var form = root.closest('form');
    var manual = form.querySelector('[data-transaction-manual-fields]');
    var submit = form.querySelector('[data-transaction-submit]');
    var start = root.querySelector('[data-dictation-start]');
    var stop = root.querySelector('[data-dictation-stop]');
    var cancel = root.querySelector('[data-dictation-cancel]');
    var status = root.querySelector('[data-dictation-status]');
    var partial = root.querySelector('[data-dictation-partial]');
    var draftsRoot = root.querySelector('[data-dictation-drafts]');
    var accountField = manual.querySelector('[name="account"]');
    var accountNames = Array.prototype.map.call(accountField.options, function (option) { return option.value; });
    var originalAction = form.getAttribute('action');
    var originalHXPost = form.getAttribute('hx-post');
    var originalSubmitDisabled = submit.disabled;
    var session = null;
    var state = { movements: [], finish_requested: false };

    function setStatus(message, isError) {
      status.textContent = message || '';
      status.hidden = !message;
      status.classList.toggle('form-error', Boolean(isError));
    }

    function setPartial(text) {
      partial.textContent = text || '';
      partial.hidden = !text;
    }

    function stopMedia() {
      if (!session) {
        return;
      }
      if (session.stream) {
        session.stream.getTracks().forEach(function (track) { track.stop(); });
      }
      if (session.node) {
        session.node.disconnect();
      }
      if (session.source) {
        session.source.disconnect();
      }
      if (session.audioContext) {
        session.audioContext.close().catch(function () {});
      }
      session.stream = null;
      session.node = null;
      session.source = null;
      session.audioContext = null;
    }

    function cleanup() {
      stopMedia();
      if (session && session.socket && session.socket.readyState < WebSocket.CLOSING) {
        session.socket.close();
      }
      session = null;
      root.classList.remove('is-recording');
    }

    function beginUI() {
      manual.disabled = true;
      submit.hidden = true;
      submit.disabled = true;
      start.hidden = true;
      stop.hidden = false;
      stop.disabled = false;
      cancel.hidden = false;
      root.classList.add('is-recording');
      draftsRoot.replaceChildren();
      state = { movements: [], finish_requested: false };
      setPartial('');
      setStatus('Preparazione del microfono...', false);
    }

    function restoreManual() {
      cleanup();
      state = { movements: [], finish_requested: false };
      draftsRoot.replaceChildren();
      manual.disabled = false;
      form.setAttribute('action', originalAction);
      form.setAttribute('hx-post', originalHXPost);
      submit.textContent = 'Salva';
      submit.hidden = false;
      submit.disabled = originalSubmitDisabled;
      start.hidden = false;
      stop.hidden = true;
      stop.disabled = false;
      cancel.hidden = true;
      setPartial('');
      setStatus('', false);
    }

    function appendTextField(card, labelText, name, value, full, required, disabled) {
      var label = document.createElement('label');
      if (full) {
        label.className = 'full';
      }
      label.appendChild(document.createTextNode(labelText));
      var input = document.createElement('input');
      input.type = 'text';
      input.name = name;
      input.value = value || '';
      input.disabled = disabled;
      input.required = Boolean(required);
      if (name === 'amount') {
        input.inputMode = 'decimal';
      }
      label.appendChild(input);
      card.appendChild(label);
    }

    function renderDrafts(finished) {
      draftsRoot.replaceChildren();
      state.movements.forEach(function (draft, index) {
        var card = document.createElement('article');
        card.className = 'dictation-draft';

        var head = document.createElement('div');
        head.className = 'dictation-draft__head';
        var title = document.createElement('strong');
        title.textContent = 'Movimento ' + (index + 1);
        head.appendChild(title);
        if (finished) {
          var remove = document.createElement('button');
          remove.type = 'button';
          remove.className = 'dictation-draft__remove';
          remove.textContent = 'Rimuovi';
          remove.addEventListener('click', function () {
            state.movements.splice(index, 1);
            renderDrafts(true);
            updateSubmit();
          });
          head.appendChild(remove);
        }
        card.appendChild(head);

        var id = document.createElement('input');
        id.type = 'hidden';
        id.name = 'id';
        id.value = draft.id;
        id.disabled = !finished;
        card.appendChild(id);

        var kindLabel = document.createElement('label');
        kindLabel.appendChild(document.createTextNode('Tipo'));
        var kind = document.createElement('select');
        kind.name = 'kind';
        kind.disabled = !finished;
        [['Expense', 'Uscita'], ['Income', 'Entrata']].forEach(function (choice) {
          var option = document.createElement('option');
          option.value = choice[0];
          option.textContent = choice[1];
          option.selected = draft.kind === choice[0];
          kind.appendChild(option);
        });
        kindLabel.appendChild(kind);
        card.appendChild(kindLabel);

        var accountLabel = document.createElement('label');
        accountLabel.appendChild(document.createTextNode('Conto'));
        var account = document.createElement('select');
        account.name = 'account';
        account.disabled = !finished;
        var matchedAccount = false;
        accountNames.forEach(function (name) {
          var option = document.createElement('option');
          option.value = name;
          option.textContent = name;
          option.selected = name.toLowerCase() === String(draft.account || '').toLowerCase();
          matchedAccount = matchedAccount || option.selected;
          account.appendChild(option);
        });
        if (!matchedAccount && draft.account) {
          var unknown = document.createElement('option');
          unknown.value = draft.account;
          unknown.textContent = draft.account + ' (da verificare)';
          unknown.selected = true;
          account.appendChild(unknown);
        }
        accountLabel.appendChild(account);
        card.appendChild(accountLabel);

        appendTextField(card, 'Data', 'date', draft.date, false, true, !finished);
        appendTextField(card, 'Importo', 'amount', draft.amount, false, true, !finished);
        appendTextField(card, 'Descrizione', 'payee', draft.payee, true, true, !finished);
        appendTextField(card, 'Categoria', 'category', draft.category, false, false, !finished);
        appendTextField(card, 'Sottocategoria', 'subcategory', draft.subcategory, false, false, !finished);
        appendTextField(card, 'Note', 'note', draft.note, true, false, !finished);

        (draft.issues || []).forEach(function (issue) {
          var warning = document.createElement('p');
          warning.className = 'dictation-draft__issue';
          warning.textContent = issue;
          card.appendChild(warning);
        });
        draftsRoot.appendChild(card);
      });
    }

    function updateSubmit() {
      submit.textContent = state.movements.length === 1 ? 'Salva movimento' : 'Salva ' + state.movements.length + ' movimenti';
      submit.disabled = state.movements.length === 0;
    }

    function finish() {
      cleanup();
      root.classList.remove('is-recording');
      stop.hidden = true;
      start.hidden = true;
      cancel.hidden = false;
      setPartial('');
      renderDrafts(true);
      form.setAttribute('action', '/dictation/confirm');
      form.setAttribute('hx-post', '/dictation/confirm');
      submit.hidden = false;
      updateSubmit();
      setStatus(state.movements.length ? 'Controlla i movimenti prima di salvarli.' : 'Non ho trovato movimenti. Puoi annullare e riprovare.', false);
    }

    function handleMessage(message) {
      if (message.type === 'ready') {
        startPCM();
      } else if (message.type === 'partial') {
        setPartial(message.text);
      } else if (message.type === 'drafts') {
        state = message.extraction || state;
        setPartial('');
        renderDrafts(false);
        setStatus('Sto aggiornando i movimenti...', false);
        if (state.finish_requested) {
          stopMedia();
        }
      } else if (message.type === 'stopped') {
        finish();
      } else if (message.type === 'error') {
        setStatus(message.message || 'La dettatura non è disponibile.', true);
        if (!message.recoverable) {
          cleanup();
          stop.hidden = true;
          start.hidden = false;
        }
      }
    }

    function startPCM() {
      var AudioContextClass = window.AudioContext || window.webkitAudioContext;
      var audioContext = new AudioContextClass();
      session.audioContext = audioContext;
      audioContext.resume().then(function () {
        return audioContext.audioWorklet.addModule('/static/pcm-worklet.js');
      }).then(function () {
        if (!session) {
          return;
        }
        var source = audioContext.createMediaStreamSource(session.stream);
        var node = new AudioWorkletNode(audioContext, 'spese-pcm-capture');
        var mute = audioContext.createGain();
        mute.gain.value = 0;
        node.port.onmessage = function (event) {
          if (!session || !session.socket || session.socket.readyState !== WebSocket.OPEN) {
            return;
          }
          var pcm = new Uint8Array(event.data);
          var frame = new Uint8Array(pcm.length + 1);
          frame[0] = 0;
          frame.set(pcm, 1);
          session.socket.send(frame);
        };
        source.connect(node);
        node.connect(mute);
        mute.connect(audioContext.destination);
        session.source = source;
        session.node = node;
        root.classList.add('is-recording');
        setStatus('Ti ascolto. Puoi correggerti mentre parli.', false);
      }).catch(function () {
        setStatus('Il browser non riesce ad avviare l’audio in tempo reale.', true);
        cleanup();
      });
    }

    function startRealtime(stream) {
      var target = new URL(root.dataset.realtimeUrl, window.location.href);
      target.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      var socket = new WebSocket(target.toString());
      socket.binaryType = 'arraybuffer';
      session = { stream: stream, socket: socket, mode: 'realtime' };
      socket.onmessage = function (event) {
        try {
          handleMessage(JSON.parse(event.data));
        } catch (_error) {
          setStatus('Risposta di dettatura non valida.', true);
        }
      };
      socket.onerror = function () {
        setStatus('Connessione alla dettatura interrotta.', true);
      };
      socket.onclose = function () {
        if (session && session.mode === 'realtime' && root.classList.contains('is-recording')) {
          cleanup();
        }
      };
    }

    function uploadFallback(blob) {
      var body = new FormData();
      var extension = blob.type.indexOf('mp4') >= 0 ? 'm4a' : 'webm';
      body.append('audio', blob, 'dettatura.' + extension);
      setStatus('Trascrizione e interpretazione in corso...', false);
      fetch(root.dataset.fallbackUrl, { method: 'POST', body: body, credentials: 'same-origin' })
        .then(function (response) {
          if (!response.ok) {
            return response.text().then(function (text) { throw new Error(text); });
          }
          return response.json();
        })
        .then(function (message) {
          handleMessage(message);
          finish();
        })
        .catch(function (error) {
          setStatus((error.message || 'Impossibile elaborare la registrazione.').trim(), true);
          stop.hidden = true;
          start.hidden = false;
        });
    }

    function startFallback(stream) {
      var chunks = [];
      var recorder = new MediaRecorder(stream);
      session = { stream: stream, recorder: recorder, mode: 'fallback' };
      recorder.ondataavailable = function (event) {
        if (event.data.size) {
          chunks.push(event.data);
        }
      };
      recorder.onstop = function () {
        var blob = new Blob(chunks, { type: recorder.mimeType });
        stopMedia();
        uploadFallback(blob);
      };
      recorder.start(500);
      root.classList.add('is-recording');
      setStatus('Ti ascolto. I movimenti appariranno dopo Termina.', false);
    }

    function begin() {
      if (!window.isSecureContext || !navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
        setStatus('Il microfono richiede una connessione HTTPS e un browser compatibile.', true);
        return;
      }
      beginUI();
      navigator.mediaDevices.getUserMedia({
        audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
      }).then(function (stream) {
        var AudioContextClass = window.AudioContext || window.webkitAudioContext;
        if (AudioContextClass && window.AudioWorkletNode && window.WebSocket) {
          startRealtime(stream);
        } else if (window.MediaRecorder) {
          startFallback(stream);
        } else {
          stream.getTracks().forEach(function (track) { track.stop(); });
          throw new Error('Questo browser non supporta la registrazione audio.');
        }
      }).catch(function (error) {
        restoreManual();
        setStatus(error.message || 'Accesso al microfono negato.', true);
      });
    }

    function stopCapture() {
      if (!session) {
        return;
      }
      stop.disabled = true;
      setStatus('Completo l’ultimo movimento...', false);
      if (session.mode === 'realtime' && session.socket.readyState === WebSocket.OPEN) {
        if (session.node) {
          session.node.port.postMessage('flush');
        }
        window.setTimeout(function () {
          if (session && session.socket && session.socket.readyState === WebSocket.OPEN) {
            session.socket.send(new Uint8Array([1]));
            stopMedia();
          }
        }, 80);
      } else if (session.mode === 'fallback' && session.recorder.state !== 'inactive') {
        session.recorder.stop();
      }
    }

    start.addEventListener('click', begin);
    stop.addEventListener('click', stopCapture);
    cancel.addEventListener('click', restoreManual);
    root._dictationCleanup = cleanup;
  }

  function initializeAll(scope) {
    (scope || document).querySelectorAll('[data-dictation]').forEach(initialize);
  }

  document.addEventListener('DOMContentLoaded', function () { initializeAll(document); });
  document.body.addEventListener('htmx:afterSwap', function (event) { initializeAll(event.detail.target); });
  document.addEventListener('spese:drawer-close', function () {
    document.querySelectorAll('[data-dictation]').forEach(function (root) {
      if (root._dictationCleanup) {
        root._dictationCleanup();
      }
    });
  });
})();
