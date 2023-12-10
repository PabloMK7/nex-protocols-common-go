package matchmake_extension

import (
	nex "github.com/PretendoNetwork/nex-go"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/globals"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/matchmake-extension"
)

func openParticipation(err error, packet nex.PacketInterface, callID uint32, gid uint32) (*nex.RMCMessage, uint32) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.Errors.Core.InvalidArgument
	}

	client := packet.Sender()

	var session *common_globals.CommonMatchmakeSession
	var ok bool
	if session, ok = common_globals.Sessions[gid]; !ok {
		return nil, nex.Errors.RendezVous.SessionVoid
	}

	if session.GameMatchmakeSession.Gathering.OwnerPID.Equals(client.PID()) {
		return nil, nex.Errors.RendezVous.PermissionDenied
	}

	session.GameMatchmakeSession.OpenParticipation = true

	rmcResponse := nex.NewRMCSuccess(nil)
	rmcResponse.ProtocolID = matchmake_extension.ProtocolID
	rmcResponse.MethodID = matchmake_extension.MethodOpenParticipation
	rmcResponse.CallID = callID

	return rmcResponse, 0
}
