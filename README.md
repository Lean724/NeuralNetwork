# NeuralNetwork

Monitor visual de tráfico de red en tiempo real representado como una **red neuronal 3D interactiva** (WebGL / Three.js). Cada dispositivo es una neurona, las conexiones son sinapsis y los paquetes viajan como impulsos luminosos codificados por protocolo.

## Características principales

- **Grafo Neuronal 3D en tiempo real:** Escena interactiva con efecto *Bloom*, rotación orbital, zoom y enfoque automático por búsqueda.
- **Pulsos adaptativos:** Pulsos individuales discretos en tráfico bajo (`ICMP`, `DNS`, `ARP`) y macro-pulsos escalados en velocidad, tamaño e intensidad durante ráfagas de alto caudal (`TCP`, `HTTPS`, `UDP`).
- **Captura en vivo (`libpcap`) + Modo Simulación:** Despliegue con `network_mode: host` y capacidades mínimas (`NET_RAW`, `NET_ADMIN`) más un generador sintético de tráfico integrado para pruebas o entornos Docker Desktop aislados.
- **Inspección de neuronas y sinapsis:** Tasas instantáneas (`Rx/Tx bps` y `pps`), dirección MAC, fabricante OUI offline, hostname, puertos y pares activos.
- **Persistencia ligera sin historial de tráfico:** SQLite (`/data/neuralnetwork.db`) almacena únicamente alias personalizados, categorías, coordenadas 3D fijadas y preferencias visuales.

## Inicio rápido con Docker Compose

```bash
cp .env.example .env
docker compose up -d --build
```

Acceder desde el navegador en:

- `http://localhost:8080` (o `http://<IP-DEL-SERVIDOR>:8080`)

## Arquitectura de red y visibilidad

- El servicio `neuralnetwork` corre con `network_mode: host` y capacidades `NET_RAW` / `NET_ADMIN` escuchando en el puerto interno `NN_INTERNAL_PORT` (`18080`), excluyendo automáticamente su propio tráfico WebSocket mediante filtro BPF.
- El servicio `neuralnetwork-gateway` actúa como puente HTTP/WebSocket en el puerto `NN_PORT` (`8080`), garantizando conectividad directa tanto en **servidores Linux nativos** como en **Docker Desktop (Windows/macOS)**.
- En redes conmutadas (switch o Wi-Fi), un host estándar observa su propio tráfico unicast y el tráfico broadcast/multicast de la LAN (`ARP`, `mDNS`, etc.). Para inspeccionar tráfico unicast entre terceros equipos, el servidor debe actuar como gateway/router o conectarse a un puerto con *Port Mirroring (SPAN)*.

## Variables de entorno

| Variable | Por defecto | Descripción |
|---|---|---|
| `NN_PORT` | `8080` | Puerto HTTP/WebSocket publicado en el navegador |
| `NN_INTERNAL_PORT` | `18080` | Puerto interno usado en `network_mode: host` |
| `NN_INTERFACE` | `auto` | Interfaz inicial (`auto`, `eth0`, `any`, etc.) |
| `NN_DEFAULT_MODE` | `live` | Modo inicial (`live` o `simulation`) |
| `NN_NODE_TTL_SEC` | `120` | TTL en segundos para ocultar nodos efímeros inactivos |
| `NN_PROMISCUOUS` | `true` | Habilita captura en modo promiscuo |

## Backup y restauración

La configuración y los alias de nodos viven en el volumen Docker `neuralnet_data` (`/data/neuralnetwork.db`):

```bash
# Backup
docker cp neuralnetwork:/data/neuralnetwork.db ./neuralnetwork.db.bak

# Restaurar
docker cp ./neuralnetwork.db.bak neuralnetwork:/data/neuralnetwork.db
docker compose restart neuralnetwork
```
