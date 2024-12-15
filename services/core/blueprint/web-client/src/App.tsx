import React, { useState } from 'react';
import { Routes, Route, useNavigate } from 'react-router-dom';

import {
  Button,
  Layout,
  Menu,
  message,
  ConfigProvider,
  ThemeConfig
} from 'antd';

import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  ClusterOutlined,
  SwapOutlined,
  RadiusSettingOutlined
} from '@ant-design/icons';

import KeyValuePage  from './pages/key_value'

const { Header, Sider, Content } = Layout;
// make {blueprint} as svg
// const logo = require('./assets/logo-white-flat.png');

const primaryBG = "#0e0e0e"

const theme: ThemeConfig = {
  token: {
    colorPrimary: "#ff873c",
    borderRadius: 2,
    colorBgBase: primaryBG,
    colorBgContainer: primaryBG,
    colorTextBase: "#A9B1B1"
  },
  components: {
    Layout: {
      siderBg: primaryBG,
      headerBg: primaryBG,
      triggerBg: primaryBG,
      bodyBg: "#201f1f",
    },
    Menu: {
      darkItemBg: primaryBG,
    },
  }
}

const App: React.FC = () => {
  const [, contextHolder] = message.useMessage();
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();

  let logo;

  if(collapsed) {
    logo = <h2 style={{textAlign: 'center', color: '#ede8e4'}}>BP</h2>
  } else {
    logo = <h2 style={{textAlign: 'center', color: '#ede8e4'}}>Blueprint</h2>
  }

  return (
    <ConfigProvider theme={theme}>
    <Layout style={{ minHeight: '100vh' }}>
      <Sider collapsible collapsed={collapsed} trigger={null} theme="dark">
        {logo}
        <Menu
          mode="inline"
          theme="dark"
          defaultSelectedKeys={['store']}
          onSelect={({ key }) => { navigate(key) }}
          items={[
            {
              key: 'store',
              icon: <SwapOutlined />,
              label: 'Key/Value',
            },
            {
              key: 'service-registry',
              icon: <RadiusSettingOutlined />,
              label: 'Service Registry',
            },
            {
              key: 'metrics',
              icon: <ClusterOutlined />,
              label: 'Metrics',
            },
          ]}
        />
      </Sider>
      <Layout>
        <Header style={{ padding: 0}}>
          <Button
            type="text"
            icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={() => setCollapsed(!collapsed)}
            style={{
              fontSize: '16px',
              width: 64,
              height: 64,
            }}
          />
        </Header>
        <Content
          style={{
            margin: '16px 16px',
          }}
        >
          {contextHolder}
          <Routes>
            {/* <Route path="/" Component={HomePage} /> */}
            <Route path="/" Component={KeyValuePage} />
            <Route path="/store" Component={KeyValuePage} />
            {/* <Route path="/upload" Component={UploadPage} /> */}
            {/* <Route path="/settings" Component={SettingsPage} /> */}
          </Routes>
        </Content>
      </Layout>
    </Layout>
  </ConfigProvider>
  );
};

export default App;
