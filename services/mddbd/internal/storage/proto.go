package storage

import pb "mddb/proto"

// DocToProto converts an internal Doc to its protobuf representation.
func DocToProto(doc *Doc) *pb.Document {
	protoMeta := make(map[string]*pb.MetaValues)
	for k, v := range doc.Meta {
		protoMeta[k] = &pb.MetaValues{Values: v}
	}
	return &pb.Document{
		Id:        doc.ID,
		Key:       doc.Key,
		Lang:      doc.Lang,
		Meta:      protoMeta,
		ContentMd: doc.ContentMD,
		AddedAt:   doc.AddedAt,
		UpdatedAt: doc.UpdatedAt,
		ExpiresAt: doc.ExpiresAt,
	}
}

// ProtoToDoc converts a protobuf Document to an internal Doc.
func ProtoToDoc(protoDoc *pb.Document) *Doc {
	meta := make(map[string][]string)
	for k, v := range protoDoc.Meta {
		meta[k] = v.Values
	}
	return &Doc{
		ID:        protoDoc.Id,
		Key:       protoDoc.Key,
		Lang:      protoDoc.Lang,
		Meta:      meta,
		ContentMD: protoDoc.ContentMd,
		AddedAt:   protoDoc.AddedAt,
		UpdatedAt: protoDoc.UpdatedAt,
		ExpiresAt: protoDoc.ExpiresAt,
	}
}
