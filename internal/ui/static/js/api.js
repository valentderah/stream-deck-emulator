// The emulator's HTTP API and event stream. Nothing here touches the page.

const base = id => `/api/devices/${id}`;
const fresh = url => `${url}?t=${Date.now()}`;

// post returns null on success or an error code.
async function post(path, body) {
  try {
    const res = await fetch(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined });
    if (res.ok) return null;
    const { code } = await res.json().catch(() => ({}));
    return code ?? 'bad_request';
  } catch {
    return 'network';
  }
}

export async function listDevices() {
  const res = await fetch('/api/devices');
  if (!res.ok) throw new Error(`devices: ${res.status}`);
  return res.json();
}

export const connect = id => post(`${base(id)}/connect`);
export const disconnect = id => post(`${base(id)}/disconnect`);
export const sendInput = (id, body) => post(`${base(id)}/input`, body);

export const keyUrl = (id, index) => fresh(`${base(id)}/key/${index}`);
export const stripUrl = id => fresh(`${base(id)}/strip`);
export const screenUrl = id => fresh(`${base(id)}/screen`);

// listen calls handlers.open when the stream connects and one handler per
// event: key(id, index), strip(id), screen(id), touchkey(id, index, rgb),
// brightness(id, percent), reset(id), link(id, status).
export function listen(handlers) {
  const events = new EventSource('/api/events');
  events.onopen = () => handlers.open();
  events.onmessage = e => {
    const m = JSON.parse(e.data);
    switch (m.kind) {
      case 'key': handlers.key(m.device, m.index); break;
      case 'strip': handlers.strip(m.device); break;
      case 'screen': handlers.screen(m.device); break;
      case 'touchkey': handlers.touchkey(m.device, m.index, m.value); break;
      case 'brightness': handlers.brightness(m.device, m.value); break;
      case 'reset': handlers.reset(m.device); break;
      case 'link': handlers.link(m.device, m.link); break;
    }
  };
}
