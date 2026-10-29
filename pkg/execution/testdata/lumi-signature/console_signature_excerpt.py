import re
import json
# --- the signature rule, as internal/metrics/signatures.go -----------------
A = re.ASCII
INCONCLUSIVE = re.compile(r'^inconclusive:\s*([a-z_]+)', A)
STATUS_IN_TEXT = re.compile(r'(?:\bhttp|\bstatus|"code")[ :="]*([1-5][0-9][0-9])\b', A)
STATUS_IN_PARENS = re.compile(r'^[a-z_]+ \(([1-5][0-9][0-9])\):', A)
STATUS_AS_RUNE = re.compile(r'^[A-Za-z_]+ \((.)\):', A)
CODE_PREFIX = re.compile(r'^[a-z_]+ \([^)]{0,8}\):\s*', A)
JSON_MESSAGE = re.compile(r'"message"\s*:\s*"((?:[^"\\]|\\.)*)"?', A)
ERROR_LINE = re.compile(r'^\s*error:\s*', A | re.I)
URL = re.compile(r'[a-z][a-z0-9+.-]*://\S+', A)
PATH = re.compile(r'(?:~|\.{1,2})?(?:/[\w.@+-]+){2,}/?', A)
HEX = re.compile(r'\b[0-9a-f]{6,}\b|\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b', A)
NUM = re.compile(r'\d+', A)
SPACE = re.compile(r'\s+', A)
MESSAGE_LIMIT = 120


def signature_class(status, text):
    lower = (text or '').strip().lower()
    m = INCONCLUSIVE.match(lower)
    if m:
        return 'inconclusive:' + m.group(1)
    if status == 'inconclusive':
        return 'inconclusive'
    m = STATUS_IN_PARENS.match(lower)
    if m:
        return 'http-' + m.group(1)
    m = STATUS_AS_RUNE.match((text or '').strip())
    if m and 100 <= ord(m.group(1)) <= 599:
        return 'http-%d' % ord(m.group(1))
    m = STATUS_IN_TEXT.search(lower)
    if m:
        return 'http-' + m.group(1)
    if 'deadline exceeded' in lower or 'timed out' in lower or 'timeout' in lower:
        return 'timeout'
    return 'error'


def signature_message(text):
    text = (text or '').strip()
    m = INCONCLUSIVE.match(text.lower())
    if m:
        text = text[m.end():]
    msg = CODE_PREFIX.sub('', text, count=1)
    m = JSON_MESSAGE.search(text)
    if m:
        try:
            msg = json.loads('"' + m.group(1) + '"')
        except ValueError:
            msg = m.group(1)
    msg = msg.replace('↵', '\n')
    first = last = ''
    for line in msg.split('\n'):
        line = line.strip()
        if line.strip('-=^ .') == '':
            continue
        if not first:
            first = line
        if ERROR_LINE.match(line):
            last = ERROR_LINE.sub('', line, count=1)
    key = (last or first).lower()
    key = URL.sub('<url>', key)
    key = PATH.sub('<path>', key)
    key = HEX.sub('#', key)
    key = NUM.sub('#', key)
    key = SPACE.sub(' ', key).strip(' .:;,')
    if len(key) > MESSAGE_LIMIT:
        key = key[:MESSAGE_LIMIT].strip()
    return key


def signature(status, text):
    return dict(cls=signature_class(status, text), message=signature_message(text))


def replay(capture, endpoint, log=lambda *_: None):
    request = capture.get('request')
    if not isinstance(request, dict) or not isinstance(request.get('messages'), list):
        raise ValueError('the capture holds no request with messages')
    status = 'inconclusive' if capture.get('outcome') == 'inconclusive' else 'failed'
    target = signature(status, capture.get('error') or '')
    report = dict(target=target, reproduced=False, partial=False, attempts=[], passes=[], requests=0)

    def same(text):
        got = signature('failed', text)
        return got['cls'] == target['cls'] and got['message'] == target['message']
