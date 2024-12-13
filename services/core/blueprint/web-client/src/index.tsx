import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter as Router } from 'react-router-dom';
import { createConnectTransport } from '@connectrpc/connect-web';
import { TransportProvider } from '@connectrpc/connect-query';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { EventListener, EventsProvider } from './services/consumer';


import { createRegistry, IMessageTypeRegistry, createRegistryFromDescriptors, ServiceOptions, JsonReadOptions, JsonFormat } from '@bufbuild/protobuf';
import { ListRequest, ListResponse } from 'api/core/registry/key_value/v1/service_pb'
import { Value } from 'api/core/registry/key_value/v1/models_pb'
import { Route } from 'api/core/control_plane/networking/v1/service_pb'

import './index.css';

import * as Config from './utils/config';
import App from './App';


const reg: IMessageTypeRegistry = createRegistry(
    Value,
    ListRequest
);

export const transport = createConnectTransport({
  baseUrl: Config.BASE_URL,
  jsonOptions: {
    typeRegistry: reg
  },
});

const queryClient = new QueryClient();


const root = ReactDOM.createRoot(
  document.getElementById('root') as HTMLElement
);
root.render(
  <React.StrictMode>
    <TransportProvider transport={transport}>
      <EventsProvider>
        <QueryClientProvider client={queryClient}>
          <Router>
            <App />
          </Router>
        </QueryClientProvider>
      </EventsProvider>
    </TransportProvider>
  </React.StrictMode>
);
