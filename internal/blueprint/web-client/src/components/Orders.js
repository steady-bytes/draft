import * as React from 'react';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import Title from './Title';

// Generate Order Data
function createData(id, name, status, state) {
  return { id, name, status, state};
}

const rows = [
  createData(
    0,
    'CST - US - South 1',
    'Online',
    'Voter',
  ),
  createData(
    1,
    'MNT - US - North 2',
    'Online',
    'Leader',
  ),
  createData(
    2, 
    'MNT - US - North 1', 
    'Online', 
    'Voter'
  ),
  createData(
    3,
    'PST - US - North 1',
    'Offline',
    'Abandoned',
  ),
  createData(
    4,
    'PST - US - South 1',
    'Online',
    'Voter',
  ),
];

function preventDefault(event) {
  event.preventDefault();
}

export default function Orders() {
  return (
    <React.Fragment>
      <Title>Cluster Nodes</Title>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Name</TableCell>
            <TableCell>Online</TableCell>
            <TableCell align="right">Node State</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.id}>
              <TableCell>{row.name}</TableCell>
              <TableCell>{row.status}</TableCell>
              <TableCell align="right">{row.state}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </React.Fragment>
  );
}