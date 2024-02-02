// Search dialog: Search key or search value in a key
// Table below matches search criteria
// Display List(25 at a time-- arrray)
// Filter endpoint(key vs value), display BroadcastChannel.Generally search for a value.
// Add key / value with modal ?? Reroute back to K / V and refresh w / state(Redux)
// Snackbar OK-- no spinners
// Snackbar Err and handle redux (sad path -- handle later)

import React, { useState } from "react";
import { useSelector, useDispatch } from "react-redux";

import { useGetValuesQuery } from "../services/key_value_rpc";
import { decrement, increment, incrementByAmount } from "../store/counter";

import {
  GetRequest,
  GetResponse,
  GetFilter,
} from "../grpc/registry/key_value/v1/service_pb";
import { createImmutableStateInvariantMiddleware } from "@reduxjs/toolkit";
import Button from "../components/Button";
import Title from "../components/Title";
import Search from "../components/Search";

export default function KeyValuesPage() {
  const count = useSelector((state) => state.counter.value);
  const dispatch = useDispatch();

  const {
    data: GetValue,
    error: GetValueError,
    isLoading: GetValueIsLoading,
  } = useGetValuesQuery({
    key: "0e7ef876-52d8-42ac-a366-01db3ddb7623",
    filter: GetFilter[2],
  });

  const clickApi = () => {
    console.log(GetValue);
  };

  const [selectedOption, setSelectedOption] = useState(null);

  const handleSelect = (option) => {
    setSelectedOption(option);
  };

  const options = [
    { label: "Option 1", value: "option1" },
    { label: "Option 2", value: "option2" },
    { label: "Option 3", value: "option3" },
    { label: "Option 4", value: "option4" },
    { label: "Option 5", value: "option5" },
    { label: "Option 6", value: "option6" },
    { label: "Option 7", value: "option7" },
    { label: "Option 8", value: "option8" },
    { label: "Option 9", value: "option9" },
  ];

  return (
    <div className="keyvalue-container">
      <div>
        <Search options={options} onSelect={handleSelect} />
      </div>

      <div className="card">
        <Title text="Counter RTK Test:" />
        <span className="keyvalue-rtkcounter">{count}</span>
        <br />
        <div className="keyvalue-cardfooter">
          <Button
            type="outline"
            text="Increment"
            onClick={() => dispatch(increment())}
          />
          <br />
          <Button
            text="Decrement"
            type="outline"
            onClick={() => dispatch(decrement())}
          />
          <br />
          <Button
            text="Add 10"
            type="outline"
            onClick={() => dispatch(incrementByAmount(10))}
          />
        </div>
      </div>

      <div className="card">
        <Title text="Set:" />
        <div className="keyvalue-cardfooter">
          <Button type="outline" text="Set" onClick={clickApi} />
        </div>
      </div>

      <div className="card">
        <Title text="Get:" />
      </div>

      <div className="card">
        <Title text="Remove:" />
      </div>

      <div className="card">
        <Title text="List:" />
      </div>
    </div>
  );
}
