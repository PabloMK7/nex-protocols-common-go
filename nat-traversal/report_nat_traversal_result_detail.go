package nattraversal

import (
	nex "github.com/PretendoNetwork/nex-go"
	nat_traversal "github.com/PretendoNetwork/nex-protocols-go/nat-traversal"

	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/globals"
)

func reportNATTraversalResultDetail(err error, packet nex.PacketInterface, callID uint32, cid uint32, result bool, detail int32, rtt uint32) (*nex.RMCMessage, uint32) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.Errors.Core.InvalidArgument
	}

	rmcResponse := nex.NewRMCSuccess(nil)
	rmcResponse.ProtocolID = nat_traversal.ProtocolID
	rmcResponse.MethodID = nat_traversal.MethodReportNATTraversalResult
	rmcResponse.CallID = callID

	return rmcResponse, 0
}
