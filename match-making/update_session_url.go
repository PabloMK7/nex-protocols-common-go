package matchmaking

import (
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/constants"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	notifications "github.com/PretendoNetwork/nex-protocols-go/v2/notifications"
	notifications_types "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/types"
)

func (commonProtocol *CommonProtocol) updateSessionURL(err error, packet nex.PacketInterface, callID uint32, idGathering *types.PrimitiveU32, strURL *types.String) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}

	session, ok := common_globals.GetSession(idGathering.Value)
	if !ok {
		return nil, nex.NewError(nex.ResultCodes.RendezVous.SessionVoid, "change_error")
	}

	connection := packet.Sender().(*nex.PRUDPConnection)
	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)
	server := endpoint.Server

	// * Mario Kart 7 seems to set an empty strURL, so I assume this is what the method does?
	originalHost := session.GameMatchmakeSession.Gathering.HostPID
	session.GameMatchmakeSession.Gathering.HostPID = connection.PID().Copy().(*types.PID)

	if common_globals.SessionManagementDebugLog {
		common_globals.Logger.Infof("GID %d: UpdateSessionURL HOST from PID %d to PID %d", idGathering.Value, originalHost.Value(), connection.PID().Value())
	}

	retval := types.NewPrimitiveBool(true)

	rmcResponseStream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())

	retval.WriteTo(rmcResponseStream)

	rmcResponseBody := rmcResponseStream.Bytes()

	rmcResponse := nex.NewRMCSuccess(endpoint, rmcResponseBody)
	rmcResponse.ProtocolID = match_making.ProtocolID
	rmcResponse.MethodID = match_making.MethodGetSessionURLs
	rmcResponse.CallID = callID

	category := notifications.NotificationCategories.HostChanged
	subtype := notifications.NotificationSubTypes.HostChanged.None

	oEvent := notifications_types.NewNotificationEvent()
	oEvent.PIDSource = connection.PID()
	oEvent.Type = types.NewPrimitiveU32(notifications.BuildNotificationType(category, subtype))
	oEvent.Param1 = types.NewPrimitiveU32(session.GameMatchmakeSession.Gathering.ID.Value)
	oEvent.Param2 = types.NewPrimitiveU32(0) // TODO - Research what this means

	// TODO - StrParam doesn't have this value on some servers
	// * https://github.com/kinnay/NintendoClients/issues/101
	// * unixTime := time.Now()
	// * oEvent.StrParam = strconv.FormatInt(unixTime.UnixMicro(), 10)

	stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())

	oEvent.WriteTo(stream)

	rmcRequest := nex.NewRMCRequest(endpoint)
	rmcRequest.ProtocolID = notifications.ProtocolID
	rmcRequest.CallID = common_globals.CurrentMatchmakingCallID.Next()
	rmcRequest.MethodID = notifications.MethodProcessNotificationEvent
	rmcRequest.Parameters = stream.Bytes()

	rmcRequestBytes := rmcRequest.Bytes()

	target := endpoint.FindConnectionByPID(originalHost.Value())
	if target == nil {
		common_globals.Logger.Warning("Connection not found")
		return rmcResponse, nil
	}

	var messagePacket nex.PRUDPPacketInterface

	if target.DefaultPRUDPVersion == 0 {
		messagePacket, _ = nex.NewPRUDPPacketV0(server, target, nil)
	} else {
		messagePacket, _ = nex.NewPRUDPPacketV1(server, target, nil)
	}

	messagePacket.SetType(constants.DataPacket)
	messagePacket.AddFlag(constants.PacketFlagNeedsAck)
	messagePacket.AddFlag(constants.PacketFlagReliable)
	messagePacket.SetSourceVirtualPortStreamType(target.StreamType)
	messagePacket.SetSourceVirtualPortStreamID(endpoint.StreamID)
	messagePacket.SetDestinationVirtualPortStreamType(target.StreamType)
	messagePacket.SetDestinationVirtualPortStreamID(target.StreamID)
	messagePacket.SetPayload(rmcRequestBytes)

	server.Send(messagePacket)

	if commonProtocol.OnAfterUpdateSessionURL != nil {
		go commonProtocol.OnAfterUpdateSessionURL(packet, idGathering, strURL)
	}

	return rmcResponse, nil
}
