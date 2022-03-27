import * as jspb from 'google-protobuf'

import * as google_protobuf_descriptor_pb from 'google-protobuf/google/protobuf/descriptor_pb';


export class GormFileOptions extends jspb.Message {
  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): GormFileOptions.AsObject;
  static toObject(includeInstance: boolean, msg: GormFileOptions): GormFileOptions.AsObject;
  static serializeBinaryToWriter(message: GormFileOptions, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): GormFileOptions;
  static deserializeBinaryFromReader(message: GormFileOptions, reader: jspb.BinaryReader): GormFileOptions;
}

export namespace GormFileOptions {
  export type AsObject = {
  }
}

export class GormMessageOptions extends jspb.Message {
  getOrmable(): boolean;
  setOrmable(value: boolean): GormMessageOptions;

  getIncludeList(): Array<ExtraField>;
  setIncludeList(value: Array<ExtraField>): GormMessageOptions;
  clearIncludeList(): GormMessageOptions;
  addInclude(value?: ExtraField, index?: number): ExtraField;

  getTable(): string;
  setTable(value: string): GormMessageOptions;

  getMultiAccount(): boolean;
  setMultiAccount(value: boolean): GormMessageOptions;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): GormMessageOptions.AsObject;
  static toObject(includeInstance: boolean, msg: GormMessageOptions): GormMessageOptions.AsObject;
  static serializeBinaryToWriter(message: GormMessageOptions, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): GormMessageOptions;
  static deserializeBinaryFromReader(message: GormMessageOptions, reader: jspb.BinaryReader): GormMessageOptions;
}

export namespace GormMessageOptions {
  export type AsObject = {
    ormable: boolean,
    includeList: Array<ExtraField.AsObject>,
    table: string,
    multiAccount: boolean,
  }
}

export class ExtraField extends jspb.Message {
  getType(): string;
  setType(value: string): ExtraField;

  getName(): string;
  setName(value: string): ExtraField;

  getTag(): GormTag | undefined;
  setTag(value?: GormTag): ExtraField;
  hasTag(): boolean;
  clearTag(): ExtraField;

  getPackage(): string;
  setPackage(value: string): ExtraField;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ExtraField.AsObject;
  static toObject(includeInstance: boolean, msg: ExtraField): ExtraField.AsObject;
  static serializeBinaryToWriter(message: ExtraField, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ExtraField;
  static deserializeBinaryFromReader(message: ExtraField, reader: jspb.BinaryReader): ExtraField;
}

export namespace ExtraField {
  export type AsObject = {
    type: string,
    name: string,
    tag?: GormTag.AsObject,
    pb_package: string,
  }
}

export class GormFieldOptions extends jspb.Message {
  getTag(): GormTag | undefined;
  setTag(value?: GormTag): GormFieldOptions;
  hasTag(): boolean;
  clearTag(): GormFieldOptions;

  getDrop(): boolean;
  setDrop(value: boolean): GormFieldOptions;

  getHasOne(): HasOneOptions | undefined;
  setHasOne(value?: HasOneOptions): GormFieldOptions;
  hasHasOne(): boolean;
  clearHasOne(): GormFieldOptions;

  getBelongsTo(): BelongsToOptions | undefined;
  setBelongsTo(value?: BelongsToOptions): GormFieldOptions;
  hasBelongsTo(): boolean;
  clearBelongsTo(): GormFieldOptions;

  getHasMany(): HasManyOptions | undefined;
  setHasMany(value?: HasManyOptions): GormFieldOptions;
  hasHasMany(): boolean;
  clearHasMany(): GormFieldOptions;

  getManyToMany(): ManyToManyOptions | undefined;
  setManyToMany(value?: ManyToManyOptions): GormFieldOptions;
  hasManyToMany(): boolean;
  clearManyToMany(): GormFieldOptions;

  getReferenceOf(): string;
  setReferenceOf(value: string): GormFieldOptions;

  getAssociationCase(): GormFieldOptions.AssociationCase;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): GormFieldOptions.AsObject;
  static toObject(includeInstance: boolean, msg: GormFieldOptions): GormFieldOptions.AsObject;
  static serializeBinaryToWriter(message: GormFieldOptions, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): GormFieldOptions;
  static deserializeBinaryFromReader(message: GormFieldOptions, reader: jspb.BinaryReader): GormFieldOptions;
}

export namespace GormFieldOptions {
  export type AsObject = {
    tag?: GormTag.AsObject,
    drop: boolean,
    hasOne?: HasOneOptions.AsObject,
    belongsTo?: BelongsToOptions.AsObject,
    hasMany?: HasManyOptions.AsObject,
    manyToMany?: ManyToManyOptions.AsObject,
    referenceOf: string,
  }

  export enum AssociationCase { 
    ASSOCIATION_NOT_SET = 0,
    HAS_ONE = 3,
    BELONGS_TO = 4,
    HAS_MANY = 5,
    MANY_TO_MANY = 6,
  }
}

export class GormTag extends jspb.Message {
  getColumn(): string;
  setColumn(value: string): GormTag;

  getType(): string;
  setType(value: string): GormTag;

  getSize(): number;
  setSize(value: number): GormTag;

  getPrecision(): number;
  setPrecision(value: number): GormTag;

  getPrimaryKey(): boolean;
  setPrimaryKey(value: boolean): GormTag;

  getUnique(): boolean;
  setUnique(value: boolean): GormTag;

  getDefault(): string;
  setDefault(value: string): GormTag;

  getNotNull(): boolean;
  setNotNull(value: boolean): GormTag;

  getAutoIncrement(): boolean;
  setAutoIncrement(value: boolean): GormTag;

  getIndex(): string;
  setIndex(value: string): GormTag;

  getUniqueIndex(): string;
  setUniqueIndex(value: string): GormTag;

  getEmbedded(): boolean;
  setEmbedded(value: boolean): GormTag;

  getEmbeddedPrefix(): string;
  setEmbeddedPrefix(value: string): GormTag;

  getIgnore(): boolean;
  setIgnore(value: boolean): GormTag;

  getForeignkey(): string;
  setForeignkey(value: string): GormTag;

  getAssociationForeignkey(): string;
  setAssociationForeignkey(value: string): GormTag;

  getManyToMany(): string;
  setManyToMany(value: string): GormTag;

  getJointableForeignkey(): string;
  setJointableForeignkey(value: string): GormTag;

  getAssociationJointableForeignkey(): string;
  setAssociationJointableForeignkey(value: string): GormTag;

  getAssociationAutoupdate(): boolean;
  setAssociationAutoupdate(value: boolean): GormTag;

  getAssociationAutocreate(): boolean;
  setAssociationAutocreate(value: boolean): GormTag;

  getAssociationSaveReference(): boolean;
  setAssociationSaveReference(value: boolean): GormTag;

  getPreload(): boolean;
  setPreload(value: boolean): GormTag;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): GormTag.AsObject;
  static toObject(includeInstance: boolean, msg: GormTag): GormTag.AsObject;
  static serializeBinaryToWriter(message: GormTag, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): GormTag;
  static deserializeBinaryFromReader(message: GormTag, reader: jspb.BinaryReader): GormTag;
}

export namespace GormTag {
  export type AsObject = {
    column: string,
    type: string,
    size: number,
    precision: number,
    primaryKey: boolean,
    unique: boolean,
    pb_default: string,
    notNull: boolean,
    autoIncrement: boolean,
    index: string,
    uniqueIndex: string,
    embedded: boolean,
    embeddedPrefix: string,
    ignore: boolean,
    foreignkey: string,
    associationForeignkey: string,
    manyToMany: string,
    jointableForeignkey: string,
    associationJointableForeignkey: string,
    associationAutoupdate: boolean,
    associationAutocreate: boolean,
    associationSaveReference: boolean,
    preload: boolean,
  }
}

export class HasOneOptions extends jspb.Message {
  getForeignkey(): string;
  setForeignkey(value: string): HasOneOptions;

  getForeignkeyTag(): GormTag | undefined;
  setForeignkeyTag(value?: GormTag): HasOneOptions;
  hasForeignkeyTag(): boolean;
  clearForeignkeyTag(): HasOneOptions;

  getAssociationForeignkey(): string;
  setAssociationForeignkey(value: string): HasOneOptions;

  getAssociationAutoupdate(): boolean;
  setAssociationAutoupdate(value: boolean): HasOneOptions;

  getAssociationAutocreate(): boolean;
  setAssociationAutocreate(value: boolean): HasOneOptions;

  getAssociationSaveReference(): boolean;
  setAssociationSaveReference(value: boolean): HasOneOptions;

  getPreload(): boolean;
  setPreload(value: boolean): HasOneOptions;

  getReplace(): boolean;
  setReplace(value: boolean): HasOneOptions;

  getAppend(): boolean;
  setAppend(value: boolean): HasOneOptions;

  getClear(): boolean;
  setClear(value: boolean): HasOneOptions;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): HasOneOptions.AsObject;
  static toObject(includeInstance: boolean, msg: HasOneOptions): HasOneOptions.AsObject;
  static serializeBinaryToWriter(message: HasOneOptions, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): HasOneOptions;
  static deserializeBinaryFromReader(message: HasOneOptions, reader: jspb.BinaryReader): HasOneOptions;
}

export namespace HasOneOptions {
  export type AsObject = {
    foreignkey: string,
    foreignkeyTag?: GormTag.AsObject,
    associationForeignkey: string,
    associationAutoupdate: boolean,
    associationAutocreate: boolean,
    associationSaveReference: boolean,
    preload: boolean,
    replace: boolean,
    append: boolean,
    clear: boolean,
  }
}

export class BelongsToOptions extends jspb.Message {
  getForeignkey(): string;
  setForeignkey(value: string): BelongsToOptions;

  getForeignkeyTag(): GormTag | undefined;
  setForeignkeyTag(value?: GormTag): BelongsToOptions;
  hasForeignkeyTag(): boolean;
  clearForeignkeyTag(): BelongsToOptions;

  getAssociationForeignkey(): string;
  setAssociationForeignkey(value: string): BelongsToOptions;

  getAssociationAutoupdate(): boolean;
  setAssociationAutoupdate(value: boolean): BelongsToOptions;

  getAssociationAutocreate(): boolean;
  setAssociationAutocreate(value: boolean): BelongsToOptions;

  getAssociationSaveReference(): boolean;
  setAssociationSaveReference(value: boolean): BelongsToOptions;

  getPreload(): boolean;
  setPreload(value: boolean): BelongsToOptions;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): BelongsToOptions.AsObject;
  static toObject(includeInstance: boolean, msg: BelongsToOptions): BelongsToOptions.AsObject;
  static serializeBinaryToWriter(message: BelongsToOptions, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): BelongsToOptions;
  static deserializeBinaryFromReader(message: BelongsToOptions, reader: jspb.BinaryReader): BelongsToOptions;
}

export namespace BelongsToOptions {
  export type AsObject = {
    foreignkey: string,
    foreignkeyTag?: GormTag.AsObject,
    associationForeignkey: string,
    associationAutoupdate: boolean,
    associationAutocreate: boolean,
    associationSaveReference: boolean,
    preload: boolean,
  }
}

export class HasManyOptions extends jspb.Message {
  getForeignkey(): string;
  setForeignkey(value: string): HasManyOptions;

  getForeignkeyTag(): GormTag | undefined;
  setForeignkeyTag(value?: GormTag): HasManyOptions;
  hasForeignkeyTag(): boolean;
  clearForeignkeyTag(): HasManyOptions;

  getAssociationForeignkey(): string;
  setAssociationForeignkey(value: string): HasManyOptions;

  getPositionField(): string;
  setPositionField(value: string): HasManyOptions;

  getPositionFieldTag(): GormTag | undefined;
  setPositionFieldTag(value?: GormTag): HasManyOptions;
  hasPositionFieldTag(): boolean;
  clearPositionFieldTag(): HasManyOptions;

  getAssociationAutoupdate(): boolean;
  setAssociationAutoupdate(value: boolean): HasManyOptions;

  getAssociationAutocreate(): boolean;
  setAssociationAutocreate(value: boolean): HasManyOptions;

  getAssociationSaveReference(): boolean;
  setAssociationSaveReference(value: boolean): HasManyOptions;

  getPreload(): boolean;
  setPreload(value: boolean): HasManyOptions;

  getReplace(): boolean;
  setReplace(value: boolean): HasManyOptions;

  getAppend(): boolean;
  setAppend(value: boolean): HasManyOptions;

  getClear(): boolean;
  setClear(value: boolean): HasManyOptions;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): HasManyOptions.AsObject;
  static toObject(includeInstance: boolean, msg: HasManyOptions): HasManyOptions.AsObject;
  static serializeBinaryToWriter(message: HasManyOptions, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): HasManyOptions;
  static deserializeBinaryFromReader(message: HasManyOptions, reader: jspb.BinaryReader): HasManyOptions;
}

export namespace HasManyOptions {
  export type AsObject = {
    foreignkey: string,
    foreignkeyTag?: GormTag.AsObject,
    associationForeignkey: string,
    positionField: string,
    positionFieldTag?: GormTag.AsObject,
    associationAutoupdate: boolean,
    associationAutocreate: boolean,
    associationSaveReference: boolean,
    preload: boolean,
    replace: boolean,
    append: boolean,
    clear: boolean,
  }
}

export class ManyToManyOptions extends jspb.Message {
  getJointable(): string;
  setJointable(value: string): ManyToManyOptions;

  getForeignkey(): string;
  setForeignkey(value: string): ManyToManyOptions;

  getJointableForeignkey(): string;
  setJointableForeignkey(value: string): ManyToManyOptions;

  getAssociationForeignkey(): string;
  setAssociationForeignkey(value: string): ManyToManyOptions;

  getAssociationJointableForeignkey(): string;
  setAssociationJointableForeignkey(value: string): ManyToManyOptions;

  getAssociationAutoupdate(): boolean;
  setAssociationAutoupdate(value: boolean): ManyToManyOptions;

  getAssociationAutocreate(): boolean;
  setAssociationAutocreate(value: boolean): ManyToManyOptions;

  getAssociationSaveReference(): boolean;
  setAssociationSaveReference(value: boolean): ManyToManyOptions;

  getPreload(): boolean;
  setPreload(value: boolean): ManyToManyOptions;

  getReplace(): boolean;
  setReplace(value: boolean): ManyToManyOptions;

  getAppend(): boolean;
  setAppend(value: boolean): ManyToManyOptions;

  getClear(): boolean;
  setClear(value: boolean): ManyToManyOptions;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ManyToManyOptions.AsObject;
  static toObject(includeInstance: boolean, msg: ManyToManyOptions): ManyToManyOptions.AsObject;
  static serializeBinaryToWriter(message: ManyToManyOptions, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ManyToManyOptions;
  static deserializeBinaryFromReader(message: ManyToManyOptions, reader: jspb.BinaryReader): ManyToManyOptions;
}

export namespace ManyToManyOptions {
  export type AsObject = {
    jointable: string,
    foreignkey: string,
    jointableForeignkey: string,
    associationForeignkey: string,
    associationJointableForeignkey: string,
    associationAutoupdate: boolean,
    associationAutocreate: boolean,
    associationSaveReference: boolean,
    preload: boolean,
    replace: boolean,
    append: boolean,
    clear: boolean,
  }
}

export class AutoServerOptions extends jspb.Message {
  getAutogen(): boolean;
  setAutogen(value: boolean): AutoServerOptions;

  getTxnMiddleware(): boolean;
  setTxnMiddleware(value: boolean): AutoServerOptions;

  getWithTracing(): boolean;
  setWithTracing(value: boolean): AutoServerOptions;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): AutoServerOptions.AsObject;
  static toObject(includeInstance: boolean, msg: AutoServerOptions): AutoServerOptions.AsObject;
  static serializeBinaryToWriter(message: AutoServerOptions, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): AutoServerOptions;
  static deserializeBinaryFromReader(message: AutoServerOptions, reader: jspb.BinaryReader): AutoServerOptions;
}

export namespace AutoServerOptions {
  export type AsObject = {
    autogen: boolean,
    txnMiddleware: boolean,
    withTracing: boolean,
  }
}

export class MethodOptions extends jspb.Message {
  getObjectType(): string;
  setObjectType(value: string): MethodOptions;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): MethodOptions.AsObject;
  static toObject(includeInstance: boolean, msg: MethodOptions): MethodOptions.AsObject;
  static serializeBinaryToWriter(message: MethodOptions, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): MethodOptions;
  static deserializeBinaryFromReader(message: MethodOptions, reader: jspb.BinaryReader): MethodOptions;
}

export namespace MethodOptions {
  export type AsObject = {
    objectType: string,
  }
}

