nodes:
  - ${NODE_IP}
  - ${NODE_IP}
  - ${NODE_IP}
  - ${NODE_IP}
  - ${NODE_IP}

gossip:
  port_range:
    start: ${GOSSIP_START_PORT}
    end: ${GOSSIP_END_PORT}

ports:
  peer: ${PEER_PORT}
  client: ${CLIENT_PORT}

output_dir: ${OUTPUT_DIR}
bin_dir: ${BIN_DIR}
lib_dir: ${LIB_DIR}
