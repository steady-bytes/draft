import React, {
  ReactNode,
  createContext,
  useContext,
  useEffect,
  useState,
} from 'react';
import { createConnectTransport } from '@connectrpc/connect-web';
import { createClient } from '@connectrpc/connect';
import { Consumer } from 'api/core/message_broker/actors/v1/consumer_connect';
import { CloudEvent } from 'api/core/message_broker/actors/v1/models_pb'
import * as Config from '../utils/config';

export const SubscribeContext = React.createContext({ client: Consumer });

export const transport = createConnectTransport({
  baseUrl: Config.BASE_URL,
});

export const client = createClient(Consumer, transport);

export type ProviderValue = {
  connected: boolean;
  setConnected: React.Dispatch<React.SetStateAction<boolean>>;
  event: CloudEvent| undefined;
  setEvent: React.Dispatch<React.SetStateAction<CloudEvent| undefined>>;
};
type DefaultValue = undefined;
type ContextValue = DefaultValue | ProviderValue;

const EventContext = createContext<ContextValue>(undefined);

export function useEvents() {
  return useContext(EventContext);
}

export type Props = {
  children: ReactNode;
};

export function EventsProvider(props: Props) {
  const { children } = props;

  const [connected, setConnected] = useState(false);
  const [event, setEvent] = useState<CloudEvent>();
  const value = {
    connected,
    setConnected,
    event,
    setEvent,
  };

  return (
    <EventContext.Provider value={value}>{children}</EventContext.Provider>
  );
}

// NOTE: this will load twice because of React.StrictMode loading all components twice
export function EventListener() {
  const { setConnected } = useEvents() as ProviderValue;

  useEffect(() => {
    console.log('initializing event listener');
    const listen = async function () {
      try {
        for await (const event of client.consume({})) {
          setConnected(true);

          console.log(event);
          // setEvent(event);
        }
      } catch (err) {
        console.warn('stream failed');
        setConnected(false);
        await new Promise((f) => setTimeout(f, 1000));
      }
    };
    (async () => {
      while (true) {
        console.log('connecting to event stream');
        await listen();
      }
    })();
  });

  return <></>;
}
