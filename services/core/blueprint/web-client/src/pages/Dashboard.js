import React, { useState } from "react";
import { Routes, Route, useNavigate } from "react-router-dom";

import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  ClusterOutlined,
  SwapOutlined,
  RadiusSettingOutlined 
} from '@ant-design/icons';

import { Button, Layout, Menu, theme } from 'antd';
const { Header, Sider, Content } = Layout;

// Pages
import KeyValuePage from "../pages/KeyValue"
import ServiceRegistryPage from "./ServiceRegistry";
import MetricsPage from "./Metrics";

// Dashboard is the main component
export default function Dashboard() {
  const [collapsed, setCollapsed] = useState(false);
  const {
    token: { colorBgContainer, borderRadiusLG },
  } = theme.useToken();

  const naviage = useNavigate();

  return (
    <>
      <Layout className="root-dashboard-layout">
        <Sider trigger={null} collapsible collapsed={collapsed}>
          <div className="demo-logo-vertical" />
          <Menu
            theme="dark"
            mode="inline"
            defaultSelectedKeys={['']}
            onClick={(evt) => {
              console.log(evt)
              naviage(evt.key)
            }}
            items={[
              {
                key: '',
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
          <Header
            style={{
              padding: 0,
              background: colorBgContainer,
            }}
          >
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
              margin: '24px 16px',
              padding: 24,
              minHeight: 280,
              background: colorBgContainer,
              borderRadius: borderRadiusLG,
            }}
          >
            <Routes>
              <Route index path="/" element={<KeyValuePage />} /> 
              <Route path="/service-registry" element={<ServiceRegistryPage />} /> 
              <Route path="/metrics" element={<MetricsPage />} /> 
            </Routes>
          </Content>
        </Layout>
      </Layout>
    </>
  );
}
